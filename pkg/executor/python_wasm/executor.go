package python_wasm

/*
The python_wasm executor wraps the docker executor. The requestor will have
automatically uploaded the execution context (python files, requirements.txt) to
ipfs so that it can be mounted into the wasm runtime container.
*/

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/filecoin-project/bacalhau/pkg/executor"
	"github.com/filecoin-project/bacalhau/pkg/storage"
	"github.com/filecoin-project/bacalhau/pkg/system"
)

type Executor struct {
	Jobs []*executor.Job

	executors map[executor.EngineType]executor.Executor
}

func NewExecutor(
	cm *system.CleanupManager,
	executors map[executor.EngineType]executor.Executor,
) (*Executor, error) {
	e := &Executor{
		executors: executors,
	}
	return e, nil
}

func (e *Executor) IsInstalled(ctx context.Context) (bool, error) {
	return true, nil
}

func (e *Executor) HasStorage(ctx context.Context,
	volume storage.StorageSpec) (bool, error) {

	return true, nil
}

func (e *Executor) RunJob(ctx context.Context, job *executor.Job) (
	string, error) {
	log.Debug().Msgf("in python_wasm executor!")
	// translate language jobspec into a docker run command
	job.Spec.Docker.Image = "quay.io/bacalhau/pyodide:4f60e3d7c52b1bf210b0def2076fdc01ec7c67ad"
	job.Spec.Engine = executor.EngineDocker
	// TODO: pass in command, and have n.js interpret it and pass it on to pyodide
	return e.executors[executor.EngineDocker].RunJob(ctx, job)
}

// Compile-time check that Executor implements the Executor interface.
var _ executor.Executor = (*Executor)(nil)
