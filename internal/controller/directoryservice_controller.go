/*
Copyright 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/389ds/ds-operator/api/v1alpha1"
)

// DirectoryServiceReconciler reconciles a DirectoryService object.
type DirectoryServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dirsrv.operator.port389.org,resources=directoryservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dirsrv.operator.port389.org,resources=directoryservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dirsrv.operator.port389.org,resources=directoryservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch

// Reconcile handles DirectoryService create/update/delete events.
func (r *DirectoryServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DirectoryService instance
	ds := &operatorv1alpha1.DirectoryService{}
	if err := r.Get(ctx, req.NamespacedName, ds); err != nil {
		// CR deleted — nothing to do (owned resources garbage-collected)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling DirectoryService", "name", ds.Name, "namespace", ds.Namespace)

	// TODO: Reconcile headless Service
	// TODO: Reconcile ClusterIP Service
	// TODO: Reconcile DM password Secret (generate if not provided)
	// TODO: Reconcile StatefulSet
	// TODO: Create suffixes on first boot (post-ready)
	// TODO: Update status

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DirectoryServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.DirectoryService{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Named("directoryservice").
		Complete(r)
}
