package internal

import (
	"context"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
)

type specCacheEntry struct {
	spec         *pb.SymbolSpecification
	reportedAtMs int64
}

// SymbolSpecService gestiona especificaciones de símbolos por cuenta.
type SymbolSpecService struct {
	mu        sync.RWMutex
	specs     map[string]map[string]*specCacheEntry // account_id → canonical → entry
	repo      domain.SymbolSpecRepository
	telemetry *telemetry.Client
	metrics   *metricbundle.EchoMetrics
	whitelist []string
}

func NewSymbolSpecService(repo domain.SymbolSpecRepository, tel *telemetry.Client, metrics *metricbundle.EchoMetrics, whitelist []string) *SymbolSpecService {
	return &SymbolSpecService{
		specs:     make(map[string]map[string]*specCacheEntry),
		repo:      repo,
		telemetry: tel,
		metrics:   metrics,
		whitelist: whitelist,
	}
}

// Upsert procesa un reporte de especificaciones y actualiza caché + persistencia.
func (s *SymbolSpecService) Upsert(ctx context.Context, report *pb.SymbolSpecReport) error {
	if err := domain.ValidateSymbolSpecReport(report, s.whitelist); err != nil {
		s.telemetry.Warn(ctx, "SymbolSpecReport validation failed",
			attribute.String("account_id", report.GetAccountId()),
			attribute.String("error", err.Error()),
			attribute.Int("symbols_count", len(report.GetSymbols())),
		)
		return err
	}

	accountID := report.AccountId
	reportedAt := report.ReportedAtMs

	s.telemetry.Debug(ctx, "Processing SymbolSpecReport",
		attribute.String("account_id", accountID),
		attribute.Int("symbols_count", len(report.Symbols)),
		attribute.Int64("reported_at_ms", reportedAt),
	)
	// Actualizar caché en memoria
	s.mu.Lock()
	if s.specs[accountID] == nil {
		s.specs[accountID] = make(map[string]*specCacheEntry)
	}
	for idx, spec := range report.Symbols {
		if spec == nil {
			s.telemetry.Debug(ctx, "Skipping nil symbol specification",
				attribute.String("account_id", accountID),
				attribute.Int("index", idx),
			)
			continue
		}
		entry := s.specs[accountID][spec.CanonicalSymbol]
		if entry != nil && entry.reportedAtMs > reportedAt {
			s.telemetry.Debug(ctx, "Skipping stale symbol specification",
				attribute.String("account_id", accountID),
				attribute.String("canonical_symbol", spec.CanonicalSymbol),
				attribute.Int64("cached_reported_at_ms", entry.reportedAtMs),
				attribute.Int64("incoming_reported_at_ms", reportedAt),
			)
			continue // Stale
		}
		cloned := proto.Clone(spec).(*pb.SymbolSpecification)
		s.specs[accountID][spec.CanonicalSymbol] = &specCacheEntry{
			spec:         cloned,
			reportedAtMs: reportedAt,
		}
		s.telemetry.Debug(ctx, "Cached symbol specification",
			attribute.String("account_id", accountID),
			attribute.String("canonical_symbol", spec.CanonicalSymbol),
			attribute.String("broker_symbol", spec.BrokerSymbol),
			attribute.Int64("reported_at_ms", reportedAt),
		)
	}
	s.mu.Unlock()

	s.metrics.RecordSymbolsReported(ctx, accountID, len(report.Symbols),
		attribute.String("source", "symbol_spec_report"),
	)
	s.telemetry.Info(ctx, "Symbol specifications upserted",
		attribute.String("account_id", accountID),
		attribute.Int("symbols_count", len(report.Symbols)),
	)

	if s.repo != nil {
		if err := s.repo.UpsertSpecifications(ctx, accountID, report.Symbols, reportedAt); err != nil {
			s.telemetry.Warn(ctx, "Failed to persist symbol specifications",
				attribute.String("account_id", accountID),
				attribute.String("error", err.Error()),
			)
			return err
		}
		s.telemetry.Debug(ctx, "Symbol specifications persisted",
			attribute.String("account_id", accountID),
			attribute.Int("symbols_count", len(report.Symbols)),
			attribute.Int64("reported_at_ms", reportedAt),
		)
	}

	return nil
}

// GetSpecification retorna una copia de la especificación almacenada.
func (s *SymbolSpecService) GetSpecification(ctx context.Context, accountID, canonical string) (*pb.SymbolSpecification, int64, bool) {
	s.mu.RLock()
	accountSpecs, ok := s.specs[accountID]
	if !ok {
		s.mu.RUnlock()
		return nil, 0, false
	}
	entry, ok := accountSpecs[canonical]
	s.mu.RUnlock()
	if !ok || entry == nil {
		return nil, 0, false
	}

	return proto.Clone(entry.spec).(*pb.SymbolSpecification), entry.reportedAtMs, true
}

// GetVolumeSpec retorna la sección de volumen y su timestamp ReportedAt.
func (s *SymbolSpecService) GetVolumeSpec(ctx context.Context, accountID, canonical string) (*pb.VolumeSpec, int64, bool) {
	spec, reportedAt, ok := s.GetSpecification(ctx, accountID, canonical)
	if !ok || spec == nil || spec.Volume == nil {
		return nil, reportedAt, false
	}

	return proto.Clone(spec.Volume).(*pb.VolumeSpec), reportedAt, true
}

// IsStale indica si la especificación está vencida según el maxAge provisto.
func (s *SymbolSpecService) IsStale(accountID, canonical string, maxAge time.Duration) bool {
	if maxAge <= 0 {
		return false
	}

	s.mu.RLock()
	accountSpecs, ok := s.specs[accountID]
	if !ok {
		s.mu.RUnlock()
		return true
	}
	entry := accountSpecs[canonical]
	s.mu.RUnlock()
	if entry == nil || entry.reportedAtMs == 0 {
		return true
	}

	reportedAt := time.UnixMilli(entry.reportedAtMs)
	age := time.Since(reportedAt)
	return age > maxAge
}

// SpecAge retorna la edad de la especificación y un booleano indicando si existe.
func (s *SymbolSpecService) SpecAge(accountID, canonical string) (time.Duration, bool) {
	s.mu.RLock()
	accountSpecs, ok := s.specs[accountID]
	if !ok {
		s.mu.RUnlock()
		return 0, false
	}
	entry := accountSpecs[canonical]
	s.mu.RUnlock()
	if entry == nil || entry.reportedAtMs == 0 {
		return 0, false
	}

	return time.Since(time.UnixMilli(entry.reportedAtMs)), true
}

// Invalidate elimina especificaciones en caché para la cuenta (mantiene persistencia).
func (s *SymbolSpecService) Invalidate(accountID string) {
	s.mu.Lock()
	delete(s.specs, accountID)
	s.mu.Unlock()
}
