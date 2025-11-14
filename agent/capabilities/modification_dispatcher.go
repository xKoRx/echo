package capabilities

import (
	"context"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

// ModificationDispatcher coordina la cola de modificaciones post fill.
type ModificationDispatcher interface {
	Enqueue(ctx context.Context, order *pb.ExecuteOrder) error
	OnExecutionResult(ctx context.Context, res *pb.ExecutionResult) error
	Cleanup(tradeID string)
}
