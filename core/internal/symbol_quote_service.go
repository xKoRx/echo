package internal

import (
	"context"
	"sync"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
)

type quoteCacheEntry struct {
	quote       *pb.SymbolQuoteSnapshot
	timestampMs int64
}

// SymbolQuoteService gestiona snapshots de precios recientes por cuenta/símbolo.
type SymbolQuoteService struct {
	mu        sync.RWMutex
	quotes    map[string]map[string]*quoteCacheEntry // account → canonical → entry
	repo      domain.SymbolQuoteRepository
	telemetry *telemetry.Client
}

func NewSymbolQuoteService(repo domain.SymbolQuoteRepository, tel *telemetry.Client) *SymbolQuoteService {
	return &SymbolQuoteService{
		quotes:    make(map[string]map[string]*quoteCacheEntry),
		repo:      repo,
		telemetry: tel,
	}
}

// Record actualiza el snapshot en memoria y persiste la última muestra.
func (s *SymbolQuoteService) Record(ctx context.Context, snapshot *pb.SymbolQuoteSnapshot) error {
	if snapshot == nil {
		return nil
	}

	s.mu.Lock()
	if s.quotes[snapshot.AccountId] == nil {
		s.quotes[snapshot.AccountId] = make(map[string]*quoteCacheEntry)
	}

	entry := s.quotes[snapshot.AccountId][snapshot.CanonicalSymbol]
	if entry != nil && entry.timestampMs > snapshot.TimestampMs {
		s.mu.Unlock()
		return nil // stale update
	}
	cloned := protoCloneQuote(snapshot)
	s.quotes[snapshot.AccountId][snapshot.CanonicalSymbol] = &quoteCacheEntry{
		quote:       cloned,
		timestampMs: snapshot.TimestampMs,
	}
	s.mu.Unlock()

	s.telemetry.Debug(ctx, "Symbol quote snapshot recorded",
		attribute.String("account_id", snapshot.AccountId),
		attribute.String("canonical_symbol", snapshot.CanonicalSymbol),
		attribute.Float64("bid", snapshot.Bid),
		attribute.Float64("ask", snapshot.Ask),
	)

	if s.repo != nil {
		if err := s.repo.InsertSnapshot(ctx, snapshot); err != nil {
			s.telemetry.Warn(ctx, "Failed to persist symbol quote snapshot",
				attribute.String("account_id", snapshot.AccountId),
				attribute.String("canonical_symbol", snapshot.CanonicalSymbol),
			)
			return err
		}
	}

	return nil
}

// Get retorna una copia del snapshot más reciente si existe.
func (s *SymbolQuoteService) Get(accountID, canonical string) (*pb.SymbolQuoteSnapshot, bool) {
	s.mu.RLock()
	accountQuotes, ok := s.quotes[accountID]
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}
	entry, ok := accountQuotes[canonical]
	s.mu.RUnlock()
	if !ok || entry == nil {
		return nil, false
	}

	return protoCloneQuote(entry.quote), true
}

// Invalidate limpia el caché para la cuenta (persistencia se mantiene).
func (s *SymbolQuoteService) Invalidate(accountID string) {
	s.mu.Lock()
	delete(s.quotes, accountID)
	s.mu.Unlock()
}

func protoCloneQuote(snapshot *pb.SymbolQuoteSnapshot) *pb.SymbolQuoteSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := proto.Clone(snapshot)
	quote, ok := cloned.(*pb.SymbolQuoteSnapshot)
	if !ok {
		return nil
	}
	return quote
}
