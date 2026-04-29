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
	"crypto/rand"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/389ds/ds-operator/api/v1alpha1"
)

const (
	defaultLDAPPort    int32 = 3389
	defaultLDAPSPort   int32 = 3636
	dataVolumeName           = "ds-data"
	dataVolumePath           = "/data"
	dmSecretVolumeName       = "dm-password"
	dmSecretMountPath        = "/run/secrets/dm-password"
	dmSecretFilePath         = dmSecretMountPath + "/dm-password"

	// Phase constants for DirectoryService status.
	phaseInitializing = "Initializing"
	phaseRunning      = "Running"
	phaseDegraded     = "Degraded"

	// DM password injection modes.
	dmPasswordModeEnv  = "env"
	dmPasswordModeFile = "file"
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
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch

// Reconcile handles DirectoryService create/update/delete events.
func (r *DirectoryServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DirectoryService instance
	ds := &operatorv1alpha1.DirectoryService{}
	if err := r.Get(ctx, req.NamespacedName, ds); err != nil {
		// CR deleted - owned resources are garbage-collected via OwnerReferences
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling DirectoryService", "name", ds.Name, "namespace", ds.Namespace)

	// Reconcile all child resources. Order matters: services and secrets before StatefulSet.
	if err := r.reconcileDMPasswordSecret(ctx, ds); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling DM password secret: %w", err)
	}

	// ConfigMap updates are independent of pod template - no pod restart triggered.
	if err := r.reconcileConfigMap(ctx, ds); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling configmap: %w", err)
	}

	// Service updates (port changes) are independent of pods - no restart needed.
	if err := r.reconcileHeadlessService(ctx, ds); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling headless service: %w", err)
	}

	if err := r.reconcileService(ctx, ds); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling service: %w", err)
	}

	// StatefulSet update: replicas changes scale without restart; pod template changes
	// (image, container ports, resources, dmPasswordMode) trigger a RollingUpdate.
	if err := r.reconcileStatefulSet(ctx, ds); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling statefulset: %w", err)
	}

	if err := r.reconcileStatus(ctx, ds); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	return ctrl.Result{}, nil
}

// --- Helper functions ---

func (r *DirectoryServiceReconciler) ldapPort(ds *operatorv1alpha1.DirectoryService) int32 {
	if ds.Spec.Ports != nil && ds.Spec.Ports.LDAP != 0 {
		return ds.Spec.Ports.LDAP
	}
	return defaultLDAPPort
}

func (r *DirectoryServiceReconciler) ldapsPort(ds *operatorv1alpha1.DirectoryService) int32 {
	if ds.Spec.Ports != nil && ds.Spec.Ports.LDAPS != 0 {
		return ds.Spec.Ports.LDAPS
	}
	return defaultLDAPSPort
}

func (r *DirectoryServiceReconciler) replicas(ds *operatorv1alpha1.DirectoryService) int32 {
	if ds.Spec.Replicas != nil {
		return *ds.Spec.Replicas
	}
	return 1
}

func (r *DirectoryServiceReconciler) storageSize(ds *operatorv1alpha1.DirectoryService) resource.Quantity {
	if ds.Spec.Storage != nil && !ds.Spec.Storage.Size.IsZero() {
		return ds.Spec.Storage.Size
	}
	return resource.MustParse("1Gi")
}

func labels(ds *operatorv1alpha1.DirectoryService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "directoryservice",
		"app.kubernetes.io/instance":   ds.Name,
		"app.kubernetes.io/managed-by": "ds-operator",
	}
}

func headlessServiceName(ds *operatorv1alpha1.DirectoryService) string {
	return ds.Name + "-internal"
}

func serviceName(ds *operatorv1alpha1.DirectoryService) string {
	return ds.Name
}

func configMapName(ds *operatorv1alpha1.DirectoryService) string {
	return ds.Name + "-config"
}

func dmSecretName(ds *operatorv1alpha1.DirectoryService) string {
	if ds.Spec.DMPasswordSecretRef != nil && ds.Spec.DMPasswordSecretRef.Name != "" {
		return ds.Spec.DMPasswordSecretRef.Name
	}
	return ds.Name + "-dm-password"
}

func dmPasswordMode(ds *operatorv1alpha1.DirectoryService) string {
	if ds.Spec.DMPasswordMode == dmPasswordModeFile {
		return dmPasswordModeFile
	}
	return dmPasswordModeEnv
}

func generatePassword() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating random password: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// --- Reconcile functions ---

func (r *DirectoryServiceReconciler) reconcileDMPasswordSecret(
	ctx context.Context, ds *operatorv1alpha1.DirectoryService,
) error {
	// If user provided a secret ref, don't create one - just verify it exists
	if ds.Spec.DMPasswordSecretRef != nil && ds.Spec.DMPasswordSecretRef.Name != "" {
		existing := &corev1.Secret{}
		return r.Get(ctx, types.NamespacedName{
			Name: ds.Spec.DMPasswordSecretRef.Name, Namespace: ds.Namespace,
		}, existing)
	}

	// Auto-generate a DM password secret
	secretName := dmSecretName(ds)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: ds.Namespace}, existing)
	if err == nil {
		return nil // already exists
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	password, err := generatePassword()
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ds.Namespace,
			Labels:    labels(ds),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"dm-password": []byte(password),
		},
	}
	if err := ctrl.SetControllerReference(ds, secret, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, secret)
}

func (r *DirectoryServiceReconciler) reconcileConfigMap(
	ctx context.Context, ds *operatorv1alpha1.DirectoryService,
) error {
	desired := r.desiredConfigMap(ds)
	if err := ctrl.SetControllerReference(ds, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update if data changed
	if !equality.Semantic.DeepEqual(existing.Data, desired.Data) {
		existing.Data = desired.Data
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *DirectoryServiceReconciler) desiredConfigMap(
	ds *operatorv1alpha1.DirectoryService,
) *corev1.ConfigMap {
	// Real 389DS container environment variables.
	// See dscontainer entrypoint: https://github.com/389ds/389-ds-base
	data := map[string]string{
		// DS_STARTUP_TIMEOUT: seconds to wait for LDAPI readiness after start.
		"DS_STARTUP_TIMEOUT": "60",
	}

	// DS_SUFFIX_NAME: sets basedn in /data/config/container.inf (first boot only).
	// The container only supports a single suffix via env var.
	if len(ds.Spec.Suffixes) > 0 {
		data["DS_SUFFIX_NAME"] = ds.Spec.Suffixes[0].DN
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(ds),
			Namespace: ds.Namespace,
			Labels:    labels(ds),
		},
		Data: data,
	}
}

func (r *DirectoryServiceReconciler) reconcileHeadlessService(
	ctx context.Context, ds *operatorv1alpha1.DirectoryService,
) error {
	desired := r.desiredHeadlessService(ds)
	if err := ctrl.SetControllerReference(ds, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update ports if changed
	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) {
		existing.Spec.Ports = desired.Spec.Ports
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *DirectoryServiceReconciler) desiredHeadlessService(
	ds *operatorv1alpha1.DirectoryService,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessServiceName(ds),
			Namespace: ds.Namespace,
			Labels:    labels(ds),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labels(ds),
			Ports: []corev1.ServicePort{
				{Name: "ldap", Port: r.ldapPort(ds), TargetPort: intstr.FromString("ldap")},
				{Name: "ldaps", Port: r.ldapsPort(ds), TargetPort: intstr.FromString("ldaps")},
			},
			PublishNotReadyAddresses: true,
		},
	}
}

func (r *DirectoryServiceReconciler) reconcileService(
	ctx context.Context, ds *operatorv1alpha1.DirectoryService,
) error {
	desired := r.desiredService(ds)
	if err := ctrl.SetControllerReference(ds, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) {
		existing.Spec.Ports = desired.Spec.Ports
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *DirectoryServiceReconciler) desiredService(
	ds *operatorv1alpha1.DirectoryService,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(ds),
			Namespace: ds.Namespace,
			Labels:    labels(ds),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels(ds),
			Ports: []corev1.ServicePort{
				{Name: "ldap", Port: r.ldapPort(ds), TargetPort: intstr.FromString("ldap")},
				{Name: "ldaps", Port: r.ldapsPort(ds), TargetPort: intstr.FromString("ldaps")},
			},
		},
	}
}

func (r *DirectoryServiceReconciler) reconcileStatefulSet(
	ctx context.Context, ds *operatorv1alpha1.DirectoryService,
) error {
	desired := r.desiredStatefulSet(ds)
	if err := ctrl.SetControllerReference(ds, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	needsUpdate := false

	if existing.Spec.Replicas == nil || *existing.Spec.Replicas != *desired.Spec.Replicas {
		existing.Spec.Replicas = desired.Spec.Replicas
		needsUpdate = true
	}

	if !equality.Semantic.DeepEqual(existing.Spec.Template, desired.Spec.Template) {
		existing.Spec.Template = desired.Spec.Template
		needsUpdate = true
	}

	if needsUpdate {
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *DirectoryServiceReconciler) desiredStatefulSet(
	ds *operatorv1alpha1.DirectoryService,
) *appsv1.StatefulSet {
	replicas := r.replicas(ds)
	ldapPort := r.ldapPort(ds)
	ldapsPort := r.ldapsPort(ds)
	storageSize := r.storageSize(ds)
	lbls := labels(ds)

	container := corev1.Container{
		Name:  "dirsrv",
		Image: ds.Spec.Image,
		Ports: []corev1.ContainerPort{
			{Name: "ldap", ContainerPort: ldapPort, Protocol: corev1.ProtocolTCP},
			{Name: "ldaps", ContainerPort: ldapsPort, Protocol: corev1.ProtocolTCP},
		},
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMapName(ds)},
			}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: dataVolumeName, MountPath: dataVolumePath},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromString("ldap"),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromString("ldap"),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       15,
		},
	}

	if ds.Spec.Resources != nil {
		container.Resources = *ds.Spec.Resources
	}

	var volumes []corev1.Volume

	// DM password injection - mode determines env var vs file mount.
	switch dmPasswordMode(ds) {
	case dmPasswordModeFile:
		// File-based: mount Secret as a volume, set DS_DM_PASSWORD_FILE env var.
		// More secure - password not visible in /proc/<pid>/environ.
		//
		// NOTE: DS_DM_PASSWORD_FILE is not yet supported by the upstream dscontainer
		// entrypoint. This mode is provided for forward-compatibility - when upstream
		// adds support, users can switch to "file" mode for improved security.
		// Track upstream: https://github.com/389ds/389-ds-base
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "DS_DM_PASSWORD_FILE",
			Value: dmSecretFilePath,
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      dmSecretVolumeName,
			MountPath: dmSecretMountPath,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: dmSecretVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: dmSecretName(ds),
				},
			},
		})
	default:
		// Env-based (default): inject DS_DM_PASSWORD from Secret via secretKeyRef.
		// Supported by all 389DS container image versions. Less secure - password
		// visible in /proc/<pid>/environ and inherited by child processes.
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "DS_DM_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: dmSecretName(ds)},
					Key:                  "dm-password",
				},
			},
		})
	}

	updateStrategy := appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ds.Name,
			Namespace: ds.Namespace,
			Labels:    lbls,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName:    headlessServiceName(ds),
			Replicas:       &replicas,
			UpdateStrategy: updateStrategy,
			Selector: &metav1.LabelSelector{
				MatchLabels: lbls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: lbls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
					Volumes:    volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   dataVolumeName,
						Labels: lbls,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: storageSize,
							},
						},
					},
				},
			},
		},
	}

	// Set storageClassName if specified
	if ds.Spec.Storage != nil && ds.Spec.Storage.StorageClassName != nil {
		sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = ds.Spec.Storage.StorageClassName
	}

	return sts
}

func (r *DirectoryServiceReconciler) reconcileStatus(
	ctx context.Context, ds *operatorv1alpha1.DirectoryService,
) error {
	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}, sts)
	if err != nil {
		if apierrors.IsNotFound(err) {
			ds.Status.Phase = phaseInitializing
			ds.Status.Replicas = 0
			ds.Status.ReadyReplicas = 0
		} else {
			return err
		}
	} else {
		ds.Status.Replicas = sts.Status.Replicas
		ds.Status.ReadyReplicas = sts.Status.ReadyReplicas

		desired := r.replicas(ds)
		switch {
		case sts.Status.ReadyReplicas == desired:
			ds.Status.Phase = phaseRunning
		case sts.Status.ReadyReplicas > 0:
			ds.Status.Phase = phaseDegraded
		default:
			ds.Status.Phase = phaseInitializing
		}
	}

	// Set Available condition
	available := metav1.ConditionFalse
	reason := "NotReady"
	message := "Waiting for pods to be ready"
	if ds.Status.Phase == phaseRunning {
		available = metav1.ConditionTrue
		reason = "AllReplicasReady"
		message = fmt.Sprintf("%d/%d replicas ready", ds.Status.ReadyReplicas, ds.Status.Replicas)
	}
	meta.SetStatusCondition(&ds.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             available,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ds.Generation,
	})

	return r.Status().Update(ctx, ds)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DirectoryServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.DirectoryService{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Named("directoryservice").
		Complete(r)
}
