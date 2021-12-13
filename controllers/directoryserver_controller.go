/*
Copyright 2021 Red Hat, Inc.

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

package controllers

import (
	"context"
	"os"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	dirsrvv1alpha1 "github.com/vashirov/ds-operator/api/v1alpha1"
)

const defaultDirsrvImage = "quay.io/vashirov/ds-container:latest"
const ldapPort = 30389

// const ldapsPort = 30636

// DirectoryServerReconciler reconciles a DirectoryServer object
type DirectoryServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dirsrv.operator.port389.org,resources=directoryservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dirsrv.operator.port389.org,resources=directoryservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dirsrv.operator.port389.org,resources=directoryservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

func (r *DirectoryServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("directoryserver", req.NamespacedName)

	// Fetch the Directory Server instance
	dirsrv := &dirsrvv1alpha1.DirectoryServer{}
	err := r.Get(ctx, req.NamespacedName, dirsrv)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("DirectoryServer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get DirectoryServer")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: dirsrv.Name, Namespace: dirsrv.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForDirectoryServer(dirsrv)
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Ensure the deployment size is the same as the spec
	size := dirsrv.Spec.Size
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Update the DirectoryServer status with the pod names
	// List the pods for this dirsrv's deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(dirsrv.Namespace),
		client.MatchingLabels(labelsForDirectoryServer(dirsrv.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "DirectoryServer.Namespace", dirsrv.Namespace, "DirectoryServer.Name", dirsrv.Name)
		return ctrl.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, dirsrv.Status.Nodes) {
		dirsrv.Status.Nodes = podNames
		err := r.Status().Update(ctx, dirsrv)
		if err != nil {
			log.Error(err, "Failed to update DirectoryServer status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// deploymentForDirectoryServer returns a directoryserver Deployment object
// TODO: add service account and volume mounts
func (r *DirectoryServerReconciler) deploymentForDirectoryServer(m *dirsrvv1alpha1.DirectoryServer) *appsv1.Deployment {
	ls := labelsForDirectoryServer(m.Name)
	replicas := m.Spec.Size
	dsImg := os.Getenv("RELATED_IMAGE_DIRSRV")
	if dsImg == "" {
		dsImg = defaultDirsrvImage
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: dsImg,
						Name:  "ds-container",
						Ports: []corev1.ContainerPort{{
							ContainerPort: ldapPort,
							Name:          "dirsrv",
						}},
					}},
				},
			},
		},
	}
	// Set DirectoryServer instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// labelsForDirectoryServer returns the labels for selecting the resources
// belonging to the given directoryserver CR name.
func labelsForDirectoryServer(name string) map[string]string {
	return map[string]string{"app": "directoryserver", "directoryserver_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

func (r *DirectoryServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dirsrvv1alpha1.DirectoryServer{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
