package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/bacalhau-project/bacalhau/pkg/models"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/rs/zerolog/log"

	"github.com/bacalhau-project/bacalhau/pkg/routing"
)

// TODO: replace the manual and lazy eviction with a more efficient caching library
type nodeInfoWrapper struct {
	models.NodeInfo
	evictAt time.Time
}

type NodeInfoStoreParams struct {
	TTL time.Duration
}

type NodeInfoStore struct {
	ttl             time.Duration
	nodeInfoMap     map[peer.ID]nodeInfoWrapper
	engineNodeIDMap map[string]map[peer.ID]struct{}
	mu              sync.RWMutex
}

func NewNodeInfoStore(params NodeInfoStoreParams) *NodeInfoStore {
	return &NodeInfoStore{
		ttl:             params.TTL,
		nodeInfoMap:     make(map[peer.ID]nodeInfoWrapper),
		engineNodeIDMap: make(map[string]map[peer.ID]struct{}),
	}
}

func (r *NodeInfoStore) Add(ctx context.Context, nodeInfo models.NodeInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// delete node from previous engines if it already exists to replace old engines with new ones if they've changed
	existingNodeInfo, ok := r.nodeInfoMap[nodeInfo.PeerInfo.ID]
	if ok {
		if existingNodeInfo.ComputeNodeInfo != nil {
			for _, engine := range existingNodeInfo.ComputeNodeInfo.ExecutionEngines {
				delete(r.engineNodeIDMap[engine], nodeInfo.PeerInfo.ID)
			}
		}
	} else {
		var engines []string
		if nodeInfo.ComputeNodeInfo != nil {
			engines = append(engines, nodeInfo.ComputeNodeInfo.ExecutionEngines...)
		}
		log.Ctx(ctx).Debug().Msgf("Adding new node %s to in-memory nodeInfo store with engines %v", nodeInfo.PeerInfo.ID, engines)
	}

	// TODO: use data structure that maintains nodes in descending order based on available capacity.
	if nodeInfo.ComputeNodeInfo != nil {
		for _, engine := range nodeInfo.ComputeNodeInfo.ExecutionEngines {
			if _, ok := r.engineNodeIDMap[engine]; !ok {
				r.engineNodeIDMap[engine] = make(map[peer.ID]struct{})
			}
			r.engineNodeIDMap[engine][nodeInfo.PeerInfo.ID] = struct{}{}
		}
	}

	// add or update the node info
	r.nodeInfoMap[nodeInfo.PeerInfo.ID] = nodeInfoWrapper{
		NodeInfo: nodeInfo,
		evictAt:  time.Now().Add(r.ttl),
	}

	log.Ctx(ctx).Trace().Msgf("Added node info %+v", nodeInfo)
	return nil
}

func (r *NodeInfoStore) Get(ctx context.Context, peerID peer.ID) (models.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infoWrapper, ok := r.nodeInfoMap[peerID]
	if !ok {
		return models.NodeInfo{}, routing.NewErrNodeNotFound(peerID)
	}
	if time.Now().After(infoWrapper.evictAt) {
		go r.evict(ctx, infoWrapper)
		return models.NodeInfo{}, routing.NewErrNodeNotFound(peerID)
	}
	return infoWrapper.NodeInfo, nil
}

func (r *NodeInfoStore) FindPeer(ctx context.Context, peerID peer.ID) (peer.AddrInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infoWrapper, ok := r.nodeInfoMap[peerID]
	if !ok {
		return peer.AddrInfo{}, nil
	}
	if len(infoWrapper.PeerInfo.Addrs) > 0 {
		return infoWrapper.PeerInfo, nil
	}
	return peer.AddrInfo{}, nil
}

func (r *NodeInfoStore) List(ctx context.Context) ([]models.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var nodeInfos []models.NodeInfo
	var toEvict []nodeInfoWrapper
	for _, nodeInfo := range r.nodeInfoMap {
		if time.Now().After(nodeInfo.evictAt) {
			toEvict = append(toEvict, nodeInfo)
		} else {
			nodeInfos = append(nodeInfos, nodeInfo.NodeInfo)
		}
	}
	if len(toEvict) > 0 {
		go r.evict(ctx, toEvict...)
	}
	return nodeInfos, nil
}

func (r *NodeInfoStore) ListForEngine(ctx context.Context, engine string) ([]models.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var nodeInfos []models.NodeInfo
	var toEvict []nodeInfoWrapper
	for nodeID := range r.engineNodeIDMap[engine] {
		nodeInfo := r.nodeInfoMap[nodeID]
		if time.Now().After(nodeInfo.evictAt) {
			toEvict = append(toEvict, nodeInfo)
		} else {
			nodeInfos = append(nodeInfos, nodeInfo.NodeInfo)
		}
	}
	if len(toEvict) > 0 {
		go r.evict(ctx, toEvict...)
	}
	return nodeInfos, nil
}

func (r *NodeInfoStore) Delete(ctx context.Context, peerID peer.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.doDelete(ctx, peerID)
}

func (r *NodeInfoStore) evict(ctx context.Context, infoWrappers ...nodeInfoWrapper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, infoWrapper := range infoWrappers {
		nodeInfo, ok := r.nodeInfoMap[infoWrapper.PeerInfo.ID]
		if !ok || nodeInfo.evictAt != infoWrapper.evictAt {
			return // node info already evicted or has been updated since it was scheduled for eviction
		}
		err := r.doDelete(ctx, infoWrapper.PeerInfo.ID)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("Failed to evict expired node info for peer %s", infoWrapper.PeerInfo.ID)
		}
	}
}

func (r *NodeInfoStore) doDelete(ctx context.Context, peerID peer.ID) error {
	nodeInfo, ok := r.nodeInfoMap[peerID]
	if !ok {
		return nil
	}
	for _, engine := range nodeInfo.ComputeNodeInfo.ExecutionEngines {
		delete(r.engineNodeIDMap[engine], peerID)
	}
	delete(r.nodeInfoMap, peerID)
	return nil
}

// compile time check that we implement the interface
var _ routing.NodeInfoStore = (*NodeInfoStore)(nil)
