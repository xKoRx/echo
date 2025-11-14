package internal

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

type riskPolicyCacheEntry struct {
	policy    *domain.RiskPolicy
	fetchedAt time.Time
}

type adjustableStopsCacheEntry struct {
	stops     *domain.AdjustableStops
	fetchedAt time.Time
}

// offsetProvider abstrae las operaciones requeridas de etcd para pruebas.
type offsetProvider interface {
	GetVarWithDefault(ctx context.Context, key, defaultValue string) (string, error)
}

type riskPolicyService struct {
	repo      domain.RiskPolicyRepository
	ttl       time.Duration
	telemetry *telemetry.Client
	metrics   *metricbundle.EchoMetrics

	mu    sync.RWMutex
	cache map[string]*riskPolicyCacheEntry
	offsetMu    sync.RWMutex
	offsetCache  map[string]*adjustableStopsCacheEntry
	offsetLoader offsetProvider
	onInvalidate  func(string)

	listenerMu     sync.Mutex
	listener       *pq.Listener
	listenerCancel context.CancelFunc
}

// NewRiskPolicyService crea un servicio de políticas de riesgo con caché en memoria.
func NewRiskPolicyService(repo domain.RiskPolicyRepository, ttl time.Duration, tel *telemetry.Client, metrics *metricbundle.EchoMetrics, offsetLoader offsetProvider) domain.RiskPolicyService {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}

	return &riskPolicyService{
		repo:        repo,
		ttl:         ttl,
		telemetry:   tel,
		metrics:     metrics,
		cache:       make(map[string]*riskPolicyCacheEntry),
		offsetCache:  make(map[string]*adjustableStopsCacheEntry),
		offsetLoader: offsetLoader,
	}
}

func policyCacheKey(accountID, strategyID string) string {
	return accountID + "::" + strategyID
}

func offsetCacheKey(accountID, symbol string) string {
	return accountID + "::" + symbol
}

func (s *riskPolicyService) Get(ctx context.Context, accountID, strategyID string) (*domain.RiskPolicy, error) {
	key := policyCacheKey(accountID, strategyID)

	s.mu.RLock()
	entry, ok := s.cache[key]
	s.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) <= s.ttl {
		result := entry.policy
		s.recordLookup(ctx, result, accountID, strategyID)
		return result, nil
	}

	policy, err := s.repo.Get(ctx, accountID, strategyID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[key] = &riskPolicyCacheEntry{
		policy:    policy,
		fetchedAt: time.Now(),
	}
	s.mu.Unlock()

	s.recordLookup(ctx, policy, accountID, strategyID)

	return policy, nil
}

func (s *riskPolicyService) GetAdjustableStops(ctx context.Context, accountID, symbol string) (*domain.AdjustableStops, error) {
	accountID = strings.TrimSpace(accountID)
	symbol = strings.TrimSpace(symbol)
	if accountID == "" || symbol == "" {
		return &domain.AdjustableStops{}, nil
	}

	canonicalSymbol := strings.ToUpper(symbol)
	key := offsetCacheKey(accountID, canonicalSymbol)

	s.offsetMu.RLock()
	entry, ok := s.offsetCache[key]
	s.offsetMu.RUnlock()
	if ok && time.Since(entry.fetchedAt) <= s.ttl {
		return cloneAdjustableStops(entry.stops), nil
	}

	stops, err := s.fetchAdjustableStops(ctx, accountID, canonicalSymbol)
	if err != nil {
		return nil, err
	}

	s.offsetMu.Lock()
	s.offsetCache[key] = &adjustableStopsCacheEntry{stops: cloneAdjustableStops(stops), fetchedAt: time.Now()}
	s.offsetMu.Unlock()

	return cloneAdjustableStops(stops), nil
}

func (s *riskPolicyService) fetchAdjustableStops(ctx context.Context, accountID, symbol string) (*domain.AdjustableStops, error) {
	if s.offsetLoader == nil {
		return &domain.AdjustableStops{}, nil
	}

	slKey := fmt.Sprintf("core/policies/%s/%s/sl_offset_points", accountID, symbol)
	tpKey := fmt.Sprintf("core/policies/%s/%s/tp_offset_points", accountID, symbol)

	slRaw, err := s.offsetLoader.GetVarWithDefault(ctx, slKey, "0")
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", slKey, err)
	}
	slPoints, err := parseOffsetValue(slRaw)
	if err != nil {
		s.logInvalidOffset(ctx, slKey, slRaw, err)
		slPoints = 0
	}

	tpRaw, err := s.offsetLoader.GetVarWithDefault(ctx, tpKey, "0")
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", tpKey, err)
	}
	tpPoints, err := parseOffsetValue(tpRaw)
	if err != nil {
		s.logInvalidOffset(ctx, tpKey, tpRaw, err)
		tpPoints = 0
	}

	return &domain.AdjustableStops{
		SLOffsetPoints: slPoints,
		TPOffsetPoints: tpPoints,
	}, nil
}

func parseOffsetValue(raw string) (int32, error) {
	val, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(val), nil
}

func cloneAdjustableStops(stops *domain.AdjustableStops) *domain.AdjustableStops {
	if stops == nil {
		return &domain.AdjustableStops{}
	}
	copy := *stops
	return &copy
}

func (s *riskPolicyService) logInvalidOffset(ctx context.Context, key, value string, err error) {
	if s.telemetry == nil {
		return
	}
	s.telemetry.Warn(ctx, "Invalid adjustable stop offset",
		attribute.String("etcd_key", key),
		attribute.String("value", value),
		attribute.String("error", err.Error()),
	)
}

func (s *riskPolicyService) Invalidate(accountID, strategyID string) {
	if accountID == "" {
		return
	}
	if strategyID == "" {
		s.invalidateAccount(accountID)
		return
	}

	key := policyCacheKey(accountID, strategyID)
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
	s.invalidateOffsets(accountID)
	s.emitInvalidate(accountID)
}

func (s *riskPolicyService) recordLookup(ctx context.Context, policy *domain.RiskPolicy, accountID, strategyID string) {
	if s.metrics == nil {
		return
	}
	result := "miss"
	attrs := []attribute.KeyValue{
		semconv.Echo.AccountID.String(accountID),
		semconv.Echo.Strategy.String(strategyID),
	}
	if policy != nil {
		result = "hit"
		attrs = append(attrs, semconv.Echo.PolicyType.String(string(policy.Type)))
	}

	s.metrics.RecordRiskPolicyLookup(ctx, result, attrs...)
}

// StartListener inicia un LISTEN/NOTIFY para invalidar caché de políticas.
func (s *riskPolicyService) StartListener(ctx context.Context, connStr string) error {
	if connStr == "" {
		return nil
	}

	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	if s.listener != nil {
		return nil
	}

	listener := pq.NewListener(connStr, 5*time.Second, time.Minute, nil)
	if err := listener.Listen("echo_risk_policy_updated"); err != nil {
		listener.Close()
		return err
	}

	childCtx, cancel := context.WithCancel(ctx)
	s.listener = listener
	s.listenerCancel = cancel

	go func() {
		for {
			select {
			case <-childCtx.Done():
				return
			case notification := <-listener.Notify:
				if notification == nil {
					continue
				}
				s.handleNotification(notification.Extra)
			}
		}
	}()

	go func() {
		<-childCtx.Done()
		s.listenerMu.Lock()
		if s.listener != nil {
			s.listener.Close()
			s.listener = nil
		}
		s.listenerMu.Unlock()
	}()

	return nil
}

// StopListener detiene el listener de LISTEN/NOTIFY.
func (s *riskPolicyService) StopListener() {
	s.listenerMu.Lock()
	if s.listenerCancel != nil {
		s.listenerCancel()
		s.listenerCancel = nil
	}
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	s.listenerMu.Unlock()
}

func (s *riskPolicyService) handleNotification(payload string) {
	accountID, strategyID := parseRiskPolicyPayload(payload)
	if accountID == "" {
		return
	}
	if strategyID == "" {
		s.invalidateAccount(accountID)
		return
	}
	s.Invalidate(accountID, strategyID)
}

// SetOnInvalidate registra un callback opcional.
func (s *riskPolicyService) SetOnInvalidate(cb func(string)) {
	s.mu.Lock()
	s.onInvalidate = cb
	s.mu.Unlock()
}

func (s *riskPolicyService) emitInvalidate(accountID string) {
	s.mu.RLock()
	cb := s.onInvalidate
	s.mu.RUnlock()
	if cb != nil && accountID != "" {
		cb(accountID)
	}
}

func parseRiskPolicyPayload(payload string) (string, string) {
	parts := strings.SplitN(payload, ":", 2)
	accountID := strings.TrimSpace(parts[0])
	strategyID := ""
	if len(parts) > 1 {
		strategyID = strings.TrimSpace(parts[1])
	}
	return accountID, strategyID
}

func (s *riskPolicyService) invalidateOffsets(accountID string) {
	if accountID == "" {
		return
	}
	prefix := accountID + "::"
	s.offsetMu.Lock()
	for key := range s.offsetCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.offsetCache, key)
		}
	}
	s.offsetMu.Unlock()
}

func (s *riskPolicyService) invalidateAccount(accountID string) {
	if accountID == "" {
		return
	}
	prefix := accountID + "::"
	s.mu.Lock()
	for key := range s.cache {
		if strings.HasPrefix(key, prefix) {
			delete(s.cache, key)
		}
	}
	s.mu.Unlock()

	s.invalidateOffsets(accountID)
	s.emitInvalidate(accountID)
}
