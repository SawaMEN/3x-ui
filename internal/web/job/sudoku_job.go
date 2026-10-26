package job

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type SudokuJob struct {
	ctx context.Context
}

func NewSudokuJob(ctx context.Context) *SudokuJob {
	job := &SudokuJob{ctx: ctx}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			sudoku.GetManager().StopAll()
		}()
	}
	return job
}

func (j *SudokuJob) Run() {
	if err := service.ReconcileSudoku(j.ctx); err != nil {
		logger.Warning("sudoku job: reconciliation failed:", err)
	}
}
