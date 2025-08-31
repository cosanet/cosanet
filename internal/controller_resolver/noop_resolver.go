package controller_resolver

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

type noopResolver struct {
}

func (n *noopResolver) GetControllerForUid(uid string) (*PodControllerRef, bool) {
	return nil, false
}

func (n *noopResolver) ControllerForPod(pod *corev1.Pod) (*PodControllerRef, bool) {
	return nil, false
}

func (n *noopResolver) ResolvePodControllerRef(pod *corev1.Pod) (*PodControllerRef, error) {
	return nil, errors.New("no-op resolver does not resolve pod controller references")
}

func (n *noopResolver) RemovePodControllerRef(pod *corev1.Pod) {
	// noop: nothing to remove from cache
}
