// Package internal contiene el resolver de símbolos por cuenta (i3).
package internal

import (
	"context"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"go.opentelemetry.io/otel/attribute"
)

// persistRequest representa una solicitud de persistencia async (i3).
type persistRequest struct {
	accountID    string
	mappings     []*domain.SymbolMapping
	reportedAtMs int64
}

// AccountSymbolResolver resuelve símbolos canónicos a símbolos de broker por cuenta (i3).
//
// Responsabilidades:
//   - Caché en memoria: account_id × canonical_symbol → SymbolInfo
//   - Persistencia async: canal con buffer y worker dedicado
//   - Warm-up lazy: carga desde PostgreSQL en primera miss por cuenta
//   - Resolución O(1) para hot-path
type AccountSymbolResolver struct {
	mu          sync.RWMutex
	cache       map[string]map[string]*domain.AccountSymbolInfo // accountID → canonical → AccountSymbolInfo
	repo        domain.SymbolRepository
	persistChan chan persistRequest // Buffer configurable (default: 1000)
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAccountSymbolResolver crea un nuevo resolver de símbolos por cuenta.
func NewAccountSymbolResolver(ctx context.Context, repo domain.SymbolRepository, tel *telemetry.Client, echoMetrics *metricbundle.EchoMetrics, persistBufferSize int) *AccountSymbolResolver {
	if persistBufferSize <= 0 {
		persistBufferSize = 1000 // Default
	}

	resolverCtx, cancel := context.WithCancel(ctx)

	return &AccountSymbolResolver{
		cache:       make(map[string]map[string]*domain.AccountSymbolInfo),
		repo:        repo,
		persistChan: make(chan persistRequest, persistBufferSize),
		telemetry:   tel,
		echoMetrics: echoMetrics,
		ctx:         resolverCtx,
		cancel:      cancel,
	}
}

// Start inicia el worker de persistencia async.
func (r *AccountSymbolResolver) Start() {
	r.wg.Add(1)
	go r.persistWorker()
}

// Stop detiene el resolver.
func (r *AccountSymbolResolver) Stop() {
	r.cancel()
	close(r.persistChan)
	r.wg.Wait()
}

// persistWorker procesa solicitudes de persistencia de forma async (i3).
func (r *AccountSymbolResolver) persistWorker() {
	defer r.wg.Done()

	for {
		select {
		case req, ok := <-r.persistChan:
			if !ok {
				return // Canal cerrado
			}

			if err := r.repo.UpsertAccountMapping(r.ctx, req.accountID, req.mappings, req.reportedAtMs); err != nil {
				r.telemetry.Error(r.ctx, "Failed to persist symbol mapping", err,
					attribute.String("account_id", req.accountID),
					attribute.Int("mappings_count", len(req.mappings)),
				)
			} else {
				r.telemetry.Debug(r.ctx, "Symbol mappings persisted",
					attribute.String("account_id", req.accountID),
					attribute.Int("mappings_count", len(req.mappings)),
				)
			}

		case <-r.ctx.Done():
			return
		}
	}
}

// UpsertMappings actualiza mapeos de símbolos para una cuenta (i3).
//
// Actualiza el caché en memoria (con lock) y encola persistencia async.
func (r *AccountSymbolResolver) UpsertMappings(ctx context.Context, accountID string, mappings []*domain.SymbolMapping, reportedAtMs int64) error {
	r.mu.Lock()
	// Inicializar mapa de cuenta si no existe
	if r.cache[accountID] == nil {
		r.cache[accountID] = make(map[string]*domain.AccountSymbolInfo)
	}

	// Invalidar mapeos previos y upsert nuevos (reemplazo completo)
	accountMap := r.cache[accountID]
	for canonical := range accountMap {
		delete(accountMap, canonical)
	}

	// Añadir nuevos mapeos
	for _, m := range mappings {
		accountMap[m.CanonicalSymbol] = m.ToAccountSymbolInfo()
	}
	r.mu.Unlock()

	// Encolar persistencia async (non-blocking con timeout)
	select {
	case r.persistChan <- persistRequest{accountID, mappings, reportedAtMs}:
		// Encolado exitoso
		r.echoMetrics.RecordSymbolsReported(ctx, accountID, len(mappings),
			attribute.String("source", "agent_report"),
		)
		r.telemetry.Info(ctx, "Symbol mappings upserted (i3)",
			attribute.String("account_id", accountID),
			attribute.Int("mappings_count", len(mappings)),
		)

	case <-time.After(100 * time.Millisecond):
		// Canal lleno - registrar warning pero continuar (no bloqueante)
		r.telemetry.Warn(ctx, "Persist channel full, dropping mapping update",
			attribute.String("account_id", accountID),
		)

	case <-r.ctx.Done():
		return r.ctx.Err()
	}

	return nil
}

// ResolveForAccount resuelve un símbolo canónico a símbolo de broker por cuenta (i3).
//
// Flujo:
//  1. Buscar en caché (O(1))
//  2. Si miss, intentar lazy load desde PostgreSQL
//  3. Retornar broker_symbol, AccountSymbolInfo y found flag
func (r *AccountSymbolResolver) ResolveForAccount(ctx context.Context, accountID, canonical string) (brokerSymbol string, info *domain.AccountSymbolInfo, found bool) {
	// 1. Buscar en caché
	r.mu.RLock()
	accountMap, hit := r.cache[accountID]
	r.mu.RUnlock()

	if !hit {
		// Lazy load desde PostgreSQL
		mappings, err := r.repo.GetAccountMapping(ctx, accountID)
		if err != nil {
			r.telemetry.Warn(ctx, "Failed to load account mapping from PostgreSQL",
				attribute.String("account_id", accountID),
				attribute.String("error", err.Error()),
			)
			r.echoMetrics.RecordSymbolLookup(ctx, "miss", accountID, canonical,
				attribute.String("source", "lazy_load_failed"),
			)
			return "", nil, false
		}

		if len(mappings) > 0 {
			// Actualizar caché
			r.mu.Lock()
			r.cache[accountID] = mappings
			accountMap = mappings
			r.mu.Unlock()

			r.echoMetrics.RecordSymbolsLoaded(ctx, "postgres", len(mappings),
				attribute.String("account_id", accountID),
			)
			r.telemetry.Info(ctx, "Account mappings loaded from PostgreSQL (lazy load)",
				attribute.String("account_id", accountID),
				attribute.Int("mappings_count", len(mappings)),
			)
		} else {
			// No hay datos aún - miss
			r.echoMetrics.RecordSymbolLookup(ctx, "miss", accountID, canonical,
				attribute.String("source", "lazy_load_empty"),
			)
			return "", nil, false
		}
	}

	// Buscar símbolo en caché
	info, found = accountMap[canonical]
	if found {
		r.echoMetrics.RecordSymbolLookup(ctx, "hit", accountID, canonical)
		return info.BrokerSymbol, info, true
	}

	// Miss en caché
	r.echoMetrics.RecordSymbolLookup(ctx, "miss", accountID, canonical,
		attribute.String("source", "cache_miss"),
	)
	return "", nil, false
}

// InvalidateAccount elimina todos los mapeos de una cuenta del caché (i3).
//
// Se llama cuando una cuenta se desconecta.
func (r *AccountSymbolResolver) InvalidateAccount(ctx context.Context, accountID string) error {
	r.mu.Lock()
	delete(r.cache, accountID)
	r.mu.Unlock()

	// Opcional: invalidar también en BD (opcional para i3, se puede omitir)
	// En i3 solo limpiamos caché; los datos en BD se mantienen para warm-up futuro

	r.telemetry.Info(ctx, "Account symbol mappings invalidated (i3)",
		attribute.String("account_id", accountID),
	)

	return nil
}

