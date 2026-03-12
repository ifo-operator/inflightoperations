package watch

import (
	"context"

	api "github.com/ifo-operator/inflightoperations/api/v1alpha1"
	"github.com/ifo-operator/inflightoperations/internal/evaluator"
	liberr "github.com/ifo-operator/inflightoperations/lib/error"
	"github.com/ifo-operator/inflightoperations/lib/logging"
	"github.com/ifo-operator/inflightoperations/settings"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var Settings = &settings.Settings

type Watcher struct {
	client          client.Client
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	restMapper      meta.RESTMapper
	log             logging.LevelLogger
	cache           *WatchCache
	rules           *RuleCache
	evaluator       evaluator.Evaluator
	operations      *Operations
}

func NewWatcher(client client.Client, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface, rules *RuleCache, evaluator evaluator.Evaluator, log logging.LevelLogger) *Watcher {
	cachedDiscovery := memory.NewMemCacheClient(discoveryClient)
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscovery)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		Settings.K8SInformerResync,
		metav1.NamespaceAll,
		nil,
	)
	return &Watcher{
		client:          client,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		informerFactory: factory,
		restMapper:      restMapper,
		log:             log,
		cache:           NewWatchCache(),
		rules:           rules,
		evaluator:       evaluator,
		operations: &Operations{
			client: client,
			log:    log,
		},
	}
}

// Register a new watch for a GVK.
func (r *Watcher) Register(gvk schema.GroupVersionKind) (err error) {
	r.cache.Lock()
	defer func() {
		r.cache.Unlock()
		if err != nil {
			r.log.Error(err, "Failed to register watch.", "gvk", gvk)
		}
	}()
	if r.cache.Exists(gvk) {
		r.log.V(4).Info("Watch already registered.", "gvk", gvk.String())
		return
	}
	r.log.V(4).Info("Registering watch.", "gvk", gvk.String())
	informer, err := r.makeInformer(gvk)
	if err != nil {
		return
	}
	err = r.addHandlers(informer, gvk)
	if err != nil {
		return
	}
	r.cache.Start(gvk, informer)
	r.log.V(0).Info("Watch registered.", "gvk", gvk.String())
	return
}

func (r *Watcher) Prune() {
	r.cache.Lock()
	defer r.cache.Unlock()
	r.cache.Prune(r.rules.GVKs())
}

func (r *Watcher) addHandlers(informer cache.SharedIndexInformer, gvk schema.GroupVersionKind) (err error) {
	_, err = informer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			iErr := r.handle(obj, gvk)
			if iErr != nil {
				r.log.Error(iErr, "failed to handle Add event", "gvk", gvk)
			}
		},
		UpdateFunc: func(_, obj any) {
			iErr := r.handle(obj, gvk)
			if iErr != nil {
				r.log.Error(iErr, "failed to handle Update event", "gvk", gvk)
			}
		},
		DeleteFunc: func(obj any) {
			iErr := r.handleDelete(obj, gvk)
			if iErr != nil {
				r.log.Error(iErr, "failed to handle Delete event", "gvk", gvk)
			}
		},
	})
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	return
}

func (r *Watcher) makeInformer(gvk schema.GroupVersionKind) (informer cache.SharedIndexInformer, err error) {
	mapping, err := r.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	informer = r.informerFactory.ForResource(mapping.Resource).Informer()
	return
}

func (r *Watcher) handle(obj any, gvk schema.GroupVersionKind) (err error) {
	subject, ok := obj.(*api.Subject)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), Settings.K8SAPITimeout)
	defer cancel()
	rulesets := r.rules.List(gvk)
	if len(rulesets) == 0 {
		r.log.V(4).Info("No rulesets for GVK", "gvk", gvk.String())
		return
	}

	detected := make(map[string]bool)
	results := []evaluator.RuleSetResult{}
	for _, ruleset := range rulesets {
		var result evaluator.RuleSetResult
		result, err = r.evaluator.EvaluateRuleSet(subject, &ruleset)
		if err != nil {
			r.log.Error(err, "Failed to evaluate ruleset", "ruleset", ruleset, "subject", subject.GetName(), "namespace", subject.GetNamespace())
			continue
		}
		results = append(results, result)
		for _, operation := range result.Operations {
			detected[operation] = true
		}
	}
	list, err := r.operations.List(ctx, subject)
	if err != nil {
		r.log.Error(err, "Failed to list operations", "subject", subject.GetName(), "namespace", subject.GetNamespace())
		return
	}
	for _, ifo := range list.Items {
		if !ifo.PastDebounceThreshold() {
			_, found := detected[ifo.Spec.Operation]
			if !found {
				r.markCompleted(ctx, subject, &ifo)
				r.cleanupCompleted(ctx, &ifo)
			}
		}
	}

	for _, result := range results {
		for _, operation := range result.Operations {
			op := r.operations.Build(subject, operation, result.RuleSet)
			op, err = r.operations.Ensure(ctx, op)
			if err != nil {
				r.log.Error(err, "Failed to ensure operation", "subject", subject.GetName(), "namespace", subject.GetNamespace())
				continue
			}
			op.MarkDetection(subject, []string{result.RuleSet.Name})
			err = r.client.Status().Update(ctx, op)
			if err != nil {
				err = liberr.Wrap(err)
				r.log.Error(err, "Failed to update operation status", "subject", subject.GetName(), "namespace", subject.GetNamespace())
				continue
			}
		}
	}
	return
}

func (r *Watcher) markCompleted(ctx context.Context, subject *api.Subject, ifo *api.InFlightOperation) {
	ifo.MarkCompleted(subject)
	err := r.client.Status().Update(ctx, ifo)
	if err != nil {
		err = liberr.Wrap(err)
		r.log.Error(err, "Failed to update operation status", "subject", subject.GetName(), "namespace", subject.GetNamespace(), "ifo", ifo.Name)
	}
}

func (r *Watcher) cleanupCompleted(ctx context.Context, ifo *api.InFlightOperation) {
	err := r.client.Delete(ctx, ifo)
	if err != nil {
		err = liberr.Wrap(err)
		r.log.Error(err, "Unable to cleanup completed IFO", "ifo", ifo.GetName())
	}
}

func (r *Watcher) handleDelete(obj any, _ schema.GroupVersionKind) (err error) {
	subject, ok := obj.(*api.Subject)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), Settings.K8SAPITimeout)
	defer cancel()
	err = r.operations.DeleteAll(ctx, subject)
	if err != nil {
		return
	}
	return
}
