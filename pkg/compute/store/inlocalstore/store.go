package inlocalstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/bacalhau-project/bacalhau/pkg/compute/store"
	"github.com/bacalhau-project/bacalhau/pkg/storage/util"
	"github.com/rs/zerolog/log"
)

type PersistentJobStoreParams struct {
	Store   store.ExecutionStore
	RootDir string
}
type PersistentExecutionStore struct {
	store     store.ExecutionStore
	stateFile string
	mu        sync.RWMutex
}

type JobStats struct {
	JobsCompleted uint64
}

// Check if the json file exists, and create it if it doesn't.
func createJobStatsJSONIfNotExists(rootDir string) (string, error) {
	_, err := os.Stat(rootDir)
	if err != nil {
		log.Error().Err(err).Msg("Error reading state root directory")
		return "", err
	}
	jsonFilepath := filepath.Join(rootDir, "jobStats.json")
	_, err = os.Stat(jsonFilepath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Debug().Msgf("Creating: %s", jsonFilepath)
			//Initialise JSON with counter of 0
			err = writeCounter(jsonFilepath, 0)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	return jsonFilepath, nil
}

func NewPersistentExecutionStore(params PersistentJobStoreParams) (*PersistentExecutionStore, error) {
	jsonFilepath, err := createJobStatsJSONIfNotExists(params.RootDir)
	if err != nil {
		return nil, err
	}

	return &PersistentExecutionStore{
		store:     params.Store,
		stateFile: jsonFilepath,
	}, nil
}

// CreateExecution implements store.ExecutionStore
func (proxy *PersistentExecutionStore) CreateExecution(ctx context.Context, execution store.LocalExecutionState) error {
	return proxy.store.CreateExecution(ctx, execution)
}

// DeleteExecution implements store.ExecutionStore
func (proxy *PersistentExecutionStore) DeleteExecution(ctx context.Context, id string) error {
	return proxy.store.DeleteExecution(ctx, id)
}

// GetExecution implements store.ExecutionStore
func (proxy *PersistentExecutionStore) GetExecution(ctx context.Context, id string) (store.LocalExecutionState, error) {
	return proxy.store.GetExecution(ctx, id)
}

// GetExecutionCount implements store.ExecutionStore
func (proxy *PersistentExecutionStore) GetExecutionCount(ctx context.Context, _ store.LocalExecutionStateType) (uint64, error) {
	proxy.mu.RLock()
	defer proxy.mu.RUnlock()
	return readCounter(proxy.stateFile)
}

// GetExecutionHistory implements store.ExecutionStore
func (proxy *PersistentExecutionStore) GetExecutionHistory(ctx context.Context, id string) ([]store.LocalStateHistory, error) {
	return proxy.store.GetExecutionHistory(ctx, id)
}

// GetExecutions implements store.ExecutionStore
func (proxy *PersistentExecutionStore) GetExecutions(ctx context.Context, sharedID string) ([]store.LocalExecutionState, error) {
	return proxy.store.GetExecutions(ctx, sharedID)
}

func (proxy *PersistentExecutionStore) GetLiveExecutions(ctx context.Context) ([]store.LocalExecutionState, error) {
	return proxy.store.GetLiveExecutions(ctx)
}

// UpdateExecutionState implements store.ExecutionStore
func (proxy *PersistentExecutionStore) UpdateExecutionState(ctx context.Context, request store.UpdateExecutionStateRequest) error {
	err := proxy.store.UpdateExecutionState(ctx, request)
	if err != nil {
		return err
	}
	if request.NewState == store.ExecutionStateCompleted {
		proxy.mu.Lock()
		defer proxy.mu.Unlock()
		count, err := readCounter(proxy.stateFile)
		if err != nil {
			return err
		}
		err = writeCounter(proxy.stateFile, count+1)
		if err != nil {
			return err
		}
	}
	return err
}

func (proxy *PersistentExecutionStore) Close(ctx context.Context) error {
	return proxy.store.Close(ctx)
}

func writeCounter(filepath string, count uint64) error {
	var jobStore JobStats
	jobStore.JobsCompleted += count
	bs, err := json.Marshal(jobStore)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath, bs, util.OS_USER_RW|util.OS_ALL_R)
	if err != nil {
		return err
	}
	return err
}

func readCounter(filepath string) (uint64, error) {
	jsonbs, err := os.ReadFile(filepath)
	var jobStore JobStats
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(jsonbs, &jobStore)
	if err != nil {
		return 0, err
	}
	return jobStore.JobsCompleted, nil
}

var _ store.ExecutionStore = (*PersistentExecutionStore)(nil)
