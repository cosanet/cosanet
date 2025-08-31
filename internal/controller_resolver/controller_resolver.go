package controller_resolver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kubecache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/lru"
)

// ResolverOptions contains configuration options for the Resolver.
// ParentCacheCapacity is the maximum number of parent controllers to cache (def: 750).
// PodCacheCapacity is the maximum number of pods to cache (def: 500).
// Nodename is the name of the node where the resolver is running.
type ResolverOptions struct {
	ParentCacheCapacity int
	PodCacheCapacity    int
	Nodename            string
}

const (
	orphanSentinel = "ORPHAN"
)

// PodControllerResolver is an abstract resolver type that can determine the
// top-level controller for a Pod. Both `Resolver` and `noopResolver` implement
// this interface.
type PodControllerResolver interface {
	// GetControllerForUid returns the cached controller ref for the Pod with the given UID, if present.
	GetControllerForUid(uid string) (*PodControllerRef, bool)

	// ResolvePodControllerRef resolves and caches the top-level controller for the given Pod.
	ResolvePodControllerRef(pod *corev1.Pod) (*PodControllerRef, error)

	// RemovePodControllerRef removes the cached controller ref for the given Pod.
	RemovePodControllerRef(pod *corev1.Pod)
}

// PodControllerRef is a compact reference to the controlling object of a Pod.
type PodControllerRef struct {
	UID        string
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

func getInt(val, def int) int {
	if val == 0 {
		return def
	}
	return val
}

func checkClientHasPermission(clientset kubernetes.Interface) (bool, []error) {
	ctx := context.TODO()
	var err error
	errors := []error{}

	// List Pods in all namespaces
	_, err = clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to list pods: %w", err))
	}

	// List ReplicaSets in all namespaces
	_, err = clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to list pods: %w", err))
	}

	// List Jobs in all namespaces
	_, err = clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to list pods: %w", err))
	}

	return len(errors) == 0, errors
}

// NewResolver constructs a Resolver that can determine the top-level controller
// (Deployment/StatefulSet/DaemonSet/CronJob/etc.) managing a Pod. It uses
// small in-memory LRU caches to reduce API calls to the Kubernetes apiserver.
func NewResolver(opts *ResolverOptions) PodControllerResolver {

	var config *rest.Config
	var err error

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(fmt.Errorf("failed to build config from kubeconfig: %w", err))
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(fmt.Errorf("failed to build in-cluster config: %w", err))
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(fmt.Errorf("failed to create clientset: %w", err))
	}

	// Test client capabilities
	ok, errs := checkClientHasPermission(clientset)
	if !ok {
		for _, err := range errs {
			slog.Warn("client permission error", slog.Any("error", err))
		}
		slog.Error("current resolver won't resolve any controller, please add necessary permissions (list Pods, ReplicaSets, Jobs across all namespaces)")
		return &noopResolver{}
	}

	r := &resolver{
		client: clientset,

		// 750 seems a reasonable amount to protect the api server without consuming that much RAM
		parentCache: cache.New(
			cache.AsLRU[string, *PodControllerRef](lru.WithCapacity(getInt(opts.PodCacheCapacity, 750))),
		),

		// 500 is a reasonable pods count per nodes
		// (according to kube official doc [even if you crank up the quotas])
		podCache: cache.New(
			cache.AsLRU[string, *PodControllerRef](lru.WithCapacity(getInt(opts.PodCacheCapacity, 500))),
		),
	}

	// Create a shared informer factory for all namespaces and the pod informer
	factory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := factory.Core().V1().Pods().Informer()

	// If node name is missing, don't filter on node
	allNodes := opts.Nodename != ""

	podInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
				return
			}
			if !allNodes && pod.Spec.NodeName != opts.Nodename || pod.Spec.NodeName == "" {
				return
			}
			_, err := r.ResolvePodControllerRef(pod)
			if err != nil {
				slog.Warn(
					"issue while resolving pod's controller",
					slog.String("pod", pod.Name),
					slog.String("namespace", pod.Namespace),
					slog.Any("err", err),
				)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod := oldObj.(*corev1.Pod)
			pod := newObj.(*corev1.Pod)
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
				return
			}
			if oldPod.ResourceVersion != pod.ResourceVersion {
				podHasJustBeenAssigned := oldPod.Spec.NodeName == "" && pod.Spec.NodeName != ""
				if podHasJustBeenAssigned && (pod.Spec.NodeName == opts.Nodename || allNodes) {
					_, err := r.ResolvePodControllerRef(pod)
					if err != nil {
						slog.Warn(
							"issue while resolving pod's controller",
							slog.String("pod", pod.Name),
							slog.String("namespace", pod.Namespace),
							slog.Any("err", err),
						)
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			r.RemovePodControllerRef(pod)
		},
	})

	stopCh := make(chan struct{})
	factory.Start(stopCh)

	factory.WaitForCacheSync(stopCh)
	slog.Info("Pod controller cache ready.")

	return r
}

// resolver resolves a Pod's managing controller and caches intermediate results.
type resolver struct {
	client      kubernetes.Interface
	parentCache *cache.Cache[string, *PodControllerRef]
	podCache    *cache.Cache[string, *PodControllerRef]
}

// RemovePodControllerRef evicts a cached entry for the given Pod from the pod cache.
func (r *resolver) RemovePodControllerRef(pod *corev1.Pod) {
	if pod == nil {
		return
	}
	r.podCache.Delete(generatePodCacheKey(pod))
}

func generateCacheKey(namespace string, ownerRef metav1.OwnerReference) string {
	return fmt.Sprintf(
		"owner:%s=%s=%s=%s=%s",
		ownerRef.UID,
		ownerRef.APIVersion,
		ownerRef.Kind,
		namespace,
		ownerRef.Name,
	)
}

func generatePodCacheKeyFromUID(uid string) string {
	return fmt.Sprintf(
		"pod:%s",
		uid,
	)
}
func generatePodCacheKey(pod *corev1.Pod) string {
	return generatePodCacheKeyFromUID(
		string(pod.GetUID()),
	)
}

// GetCachedPodControllerRef returns the cached controller ref for the Pod, if present.
// Return object (PodControllerRef) and if found (bool)
func (r *resolver) GetControllerForUid(uid string) (*PodControllerRef, bool) {
	if uid == "" {
		return nil, false
	}
	podKey := generatePodCacheKeyFromUID(uid)
	return r.podCache.Get(podKey)
}

// GetCachedPodControllerRef returns the cached controller ref for the Pod, if present.
// Return object (PodControllerRef) and if found (bool)
func (r *resolver) ControllerForPod(pod *corev1.Pod) (*PodControllerRef, bool) {
	if pod == nil {
		return nil, false
	}
	podKey := generatePodCacheKey(pod)
	return r.podCache.Get(podKey)
}

// ResolvePodControllerRef returns the top-level controller for the Pod, consulting
// caches first to minimize API calls. A nil pod results in an error.
func (r *resolver) ResolvePodControllerRef(pod *corev1.Pod) (*PodControllerRef, error) {
	if pod == nil {
		return nil, errors.New("nil provided pod")
	}
	podKey := generatePodCacheKey(pod)

	if cached, ok := r.podCache.Get(podKey); ok {
		slog.Debug("pod cache hit", slog.String("key", podKey))
		return cached, nil
	}

	namespace := pod.GetNamespace()
	orefs := pod.GetOwnerReferences()
	var res *PodControllerRef
	var err error

	if len(orefs) == 0 {
		slog.Debug(
			"orphan pod",
			slog.String("pod", pod.GetName()),
			slog.String("namespace", pod.GetNamespace()),
			slog.String("reason", "no owner references found"),
		)
		// Don't cache orphan pods, *could* be adopted later on
		return &PodControllerRef{
			UID:        orphanSentinel,
			APIVersion: orphanSentinel,
			Kind:       orphanSentinel,
			Namespace:  namespace,
			Name:       orphanSentinel,
		}, nil
	}

	// Check cache
	if res, found := r.podCache.Get(podKey); found {
		return res, nil
	}

	ownerRef := getControllerOwnerReference(orefs)

	switch ownerRef.Kind {
	case "ReplicaSet", "Job":
		res, err = r.getParentDetail(namespace, ownerRef)
	case "StatefulSet", "DaemonSet", "Deployment", "CronJob":
		res = &PodControllerRef{
			UID:        string(ownerRef.UID),
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Namespace:  namespace,
			Name:       ownerRef.Name,
		}
	case "Node":
		res = &PodControllerRef{
			UID:        string(ownerRef.UID),
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Namespace:  "",
			Name:       ownerRef.Name,
		}
	default:
		res, err = r.getParentDetail(namespace, ownerRef)
	}

	if err != nil {
		return nil, err
	}
	r.podCache.Set(podKey, res)
	return res, nil
}

func getControllerOwnerReference(orefs []metav1.OwnerReference) metav1.OwnerReference {
	for _, ref := range orefs {
		if ref.Controller != nil && *ref.Controller {
			return ref
		}
	}
	return orefs[0]
}

func (r *resolver) getParentDetail(namespace string, ownerRef metav1.OwnerReference) (*PodControllerRef, error) {
	var err error
	var obj metav1.Object

	cacheKey := generateCacheKey(namespace, ownerRef)
	if cached, ok := r.parentCache.Get(cacheKey); ok {
		slog.Debug("parent cache hit", slog.String("key", cacheKey))
		return cached, nil
	}

	slog.Debug(
		"parent cache miss",
		slog.String("key", cacheKey),
		slog.String("kind", ownerRef.Kind),
		slog.String("name", ownerRef.Name),
	)
	ctx := context.TODO()
	switch ownerRef.Kind {
	case "ReplicaSet":
		// Seek for the underlying deployment
		obj, err = r.client.AppsV1().ReplicaSets(namespace).Get(ctx, ownerRef.Name, metav1.GetOptions{})
	case "Job":
		// Seek for the possible CronJob
		obj, err = r.client.BatchV1().Jobs(namespace).Get(ctx, ownerRef.Name, metav1.GetOptions{})
	default:
		// Directly return the ownerRef as top-level
		res := &PodControllerRef{
			UID:        string(ownerRef.UID),
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Namespace:  namespace,
			Name:       ownerRef.Name,
		}
		r.parentCache.Set(cacheKey, res)
		return res, nil
	}

	if err != nil {
		return nil, err
	}

	parent := obj.GetOwnerReferences()
	var result *PodControllerRef
	if len(parent) == 0 {
		result = &PodControllerRef{
			UID:        string(ownerRef.UID),
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Namespace:  namespace,
			Name:       ownerRef.Name,
		}
	} else {
		controlling := getControllerOwnerReference(parent)
		result = &PodControllerRef{
			UID:        string(controlling.UID),
			APIVersion: controlling.APIVersion,
			Kind:       controlling.Kind,
			Namespace:  namespace,
			Name:       controlling.Name,
		}
	}
	r.parentCache.Set(cacheKey, result)

	return result, nil
}
