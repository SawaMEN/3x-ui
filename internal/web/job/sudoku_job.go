package job

import (
	"context"
	"errors"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type SudokuJob struct {
	ctx context.Context
}

// The server owns process shutdown. Canceling a panel-only reload must leave
// existing Sudoku sessions alive, just like the other protocol managers.
func NewSudokuJob(ctx context.Context) *SudokuJob {
	if ctx == nil {
		ctx = context.Background()
	}
	return &SudokuJob{ctx: ctx}
}

func (j *SudokuJob) Run() {
	ctx := j.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	if err := service.ReconcileSudoku(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Warning("sudoku job: reconciliation failed:", err)
	}
}
