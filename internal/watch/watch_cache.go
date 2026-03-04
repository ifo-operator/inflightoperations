package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func NewWatchCache() *WatchCache {
	return &WatchCache{
		watches: make(map[schema.GroupVersionKind]*Watch),
	}
}

// WatchCache must be locked before interacting with it.
type WatchCache struct {
	mu      sync.RWMutex
	watches map[schema.GroupVersionKind]*Watch
}

func (r *WatchCache) Lock() {
	r.mu.Lock()
}

func (r *WatchCache) Unlock() {
	r.mu.Unlock()
}

func (r *WatchCache) StartWithSync(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	w := NewWatch(gvk, informer)
	err = w.StartWithSync(ctx)
	if err != nil {
		return
	}
	r.watches[gvk] = w
	return
}

func (r *WatchCache) Start(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) {
	w := NewWatch(gvk, informer)
	w.Start()
	r.watches[gvk] = w
}

func (r *WatchCache) Stop(gvk schema.GroupVersionKind) {
	w, ok := r.watches[gvk]
	if !ok {
		return
	}
	w.Stop()
	delete(r.watches, gvk)
}

func (r *WatchCache) Exists(gvk schema.GroupVersionKind) (ok bool) {
	_, ok = r.watches[gvk]
	return
}

func (r *WatchCache) Prune(gvks []schema.GroupVersionKind) {
	keep := make(map[schema.GroupVersionKind]bool)
	for _, gvk := range gvks {
		keep[gvk] = true
	}
	for gvk := range r.watches {
		if !keep[gvk] {
			r.Stop(gvk)
		}
	}
}

func NewWatch(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) (w *Watch) {
	w = &Watch{
		gvk:      gvk,
		informer: informer,
	}
	return
}

// Watch tracks the state of a single dynamic watch
type Watch struct {
	gvk      schema.GroupVersionKind
	informer cache.SharedIndexInformer
	stopCh   chan struct{}
	running  bool
}

func (r *Watch) StartWithSync(ctx context.Context) (err error) {
	r.Start()
	if !cache.WaitForCacheSync(ctx.Done(), r.informer.HasSynced) {
		r.Stop()
		err = fmt.Errorf("failed to sync cache for GVK %s", r.gvk.String())
		return
	}
	return
}

func (r *Watch) Start() {
	r.stopCh = make(chan struct{})
	r.running = true
	go r.informer.Run(r.stopCh)
}

func (r *Watch) Stop() {
	close(r.stopCh)
	r.running = false
}
