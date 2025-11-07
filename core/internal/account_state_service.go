package internal

import (
	"context"
	"sync"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
)

type accountStateEntry struct {
	info *pb.AccountInfo
}

// AccountStateService mantiene un caché en memoria del estado de cuentas reportado por los Agents.
type AccountStateService struct {
	mu        sync.RWMutex
	accounts  map[string]*accountStateEntry
	telemetry *telemetry.Client
}

func NewAccountStateService(tel *telemetry.Client) *AccountStateService {
	return &AccountStateService{
		accounts:  make(map[string]*accountStateEntry),
		telemetry: tel,
	}
}

// Update procesa un StateSnapshot recibido desde un Agent y actualiza el caché.
func (s *AccountStateService) Update(ctx context.Context, agentID string, snapshot *pb.StateSnapshot) {
	if snapshot == nil {
		return
	}

	if len(snapshot.Accounts) == 0 {
		if s.telemetry != nil {
			s.telemetry.Info(ctx, "Account state snapshot without accounts",
				attribute.String("agent_id", agentID),
				attribute.Int64("timestamp_ms", snapshot.TimestampMs),
			)
		}
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("agent_id", agentID),
		attribute.Int("accounts", len(snapshot.Accounts)),
	}

	currencyMissing := 0

	s.mu.Lock()
	for _, account := range snapshot.Accounts {
		if account == nil || account.AccountId == "" {
			continue
		}
		clone := proto.Clone(account).(*pb.AccountInfo)
		s.accounts[account.AccountId] = &accountStateEntry{info: clone}
		attrs = append(attrs,
			semconv.Echo.AccountID.String(account.AccountId),
			attribute.String("account_currency", clone.Currency),
		)
		if clone.Currency == "" {
			currencyMissing++
		}
	}
	s.mu.Unlock()

	if currencyMissing > 0 {
		attrs = append(attrs, attribute.Int("accounts_currency_missing", currencyMissing))
	}

	if s.telemetry != nil {
		s.telemetry.Info(ctx, "Account state snapshot updated", attrs...)
	}
}

// Get retorna una copia del AccountInfo almacenado para la cuenta solicitada.
func (s *AccountStateService) Get(accountID string) (*pb.AccountInfo, bool) {
	if accountID == "" {
		return nil, false
	}

	s.mu.RLock()
	entry, ok := s.accounts[accountID]
	s.mu.RUnlock()
	if !ok || entry == nil || entry.info == nil {
		return nil, false
	}

	clone := proto.Clone(entry.info).(*pb.AccountInfo)
	return clone, true
}

// Invalidate elimina el estado de una cuenta específica del caché.
func (s *AccountStateService) Invalidate(accountID string) {
	if accountID == "" {
		return
	}

	s.mu.Lock()
	delete(s.accounts, accountID)
	s.mu.Unlock()
}
