/*
Copyright 2025.

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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mayooot/serviceaccount-quota/pkg/annotations"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	quotav1alpha1 "github.com/mayooot/serviceaccount-quota/api/v1alpha1"
)

// ServiceAccountQuotaReconciler reconciles a ServiceAccountQuota object
type ServiceAccountQuotaReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func NewServiceAccountResourceQuotaReconciler(mgr manager.Manager) *ServiceAccountQuotaReconciler {
	return &ServiceAccountQuotaReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("accountquota-controller"),
	}
}

// +kubebuilder:rbac:groups=quota.mayooot.github.io,resources=serviceaccountquotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=quota.mayooot.github.io,resources=serviceaccountquotas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=quota.mayooot.github.io,resources=serviceaccountquotas/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServiceAccountQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithName("accountquota").WithValues("name", req.NamespacedName)
	log.Info("Reconciling ServiceAccountQuota")

	// Fetch the ServiceAccountResourceQuota
	var quota quotav1alpha1.ServiceAccountQuota
	if err := r.Get(ctx, req.NamespacedName, &quota); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Copy hard quota to status.hard
	quota.Status.Hard = quota.Spec.Hard.DeepCopy()

	// Initialize used resource to zero
	used := quota.Spec.Hard.DeepCopy()
	for key := range used {
		used[key] = resource.Quantity{}
	}

	// List pods and calculate used resources
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{
		fmt.Sprintf("metadata.annotations[%s]", annotations.ServiceAccountKey): quota.Spec.ServiceAccountName,
	}); err != nil {
		return ctrl.Result{}, err
	}

	for _, pod := range podList.Items {
		namespaceName := pod.Namespace + "/" + pod.Name
		saName, hasAnnotation := pod.Annotations[annotations.ServiceAccountKey]
		if !hasAnnotation || saName != quota.Spec.ServiceAccountName {
			log.Info("ServiceAccount annotation mismatch or missing", "namespaceName", namespaceName)
			continue
		}

		// Only count resources for Pending or Running pods
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			// Count the pod itself
			if _, exists := quota.Spec.Hard["pods"]; exists {
				oldQuantity := used["pods"]
				oldQuantity.Add(resource.MustParse("1"))
				used["pods"] = oldQuantity
			}

			for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
				for resourceName, quantity := range container.Resources.Requests {
					rName := "requests." + string(resourceName)
					if _, exists := quota.Spec.Hard[corev1.ResourceName(rName)]; exists {
						oldQuantity := used[corev1.ResourceName(rName)]
						oldQuantity.Add(quantity)
						used[corev1.ResourceName(rName)] = oldQuantity
					}
				}
				for resourceName, quantity := range container.Resources.Limits {
					rName := "limits." + string(resourceName)
					if _, exists := quota.Spec.Hard[corev1.ResourceName(rName)]; exists {
						oldQuantity := used[corev1.ResourceName(rName)]
						oldQuantity.Add(quantity)
						used[corev1.ResourceName(rName)] = oldQuantity
					}
				}
			}
		}
	}

	quota.Status.Used = used

	// Generate QuotaSummary in "resource: used/hard" format
	quota.Status.QuotaSummary = formatQuotaSummary(quota.Status.Used, quota.Status.Hard)
	quota.Status.LastReconciled = &metav1.Time{Time: time.Now()}

	// Check for quota violations
	for resourceName, usedQuantity := range quota.Status.Used {
		if hardQuantity, exists := quota.Status.Hard[resourceName]; exists && usedQuantity.Cmp(hardQuantity) > 0 {
			violation := fmt.Sprintf("%s usage (%s) exceeds hard limit (%s)", resourceName, usedQuantity.String(), hardQuantity.String())
			log.Info("Quota violation detected", "violation", violation)
			r.Recorder.Event(&quota, corev1.EventTypeWarning, "QuotaExceeded", violation)
		}
	}

	// Update status
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the object to get the latest resourceVersion
		var currentQuota quotav1alpha1.ServiceAccountQuota
		if err := r.Get(ctx, req.NamespacedName, &currentQuota); err != nil {
			return err
		}
		// Apply the new status to the latest object
		currentQuota.Status = quota.Status
		// Attempt to update the status
		return r.Status().Update(ctx, &currentQuota)
	})
	if err != nil {
		log.Error(err, "Unable to update ServiceAccountResourceQuota status after retries")
		return ctrl.Result{Requeue: true}, err
	}

	log.Info("ServiceAccountResourceQuota status updated", "name", quota.Name)
	log.Info("ServiceAccountResourceQuota reconciled", "name", quota.Name)

	return ctrl.Result{}, nil
}

// formatQuotaSummary generates a string in the format "resource: used/hard, ..."
func formatQuotaSummary(used, hard corev1.ResourceList) string {
	if len(hard) == 0 {
		return "None"
	}
	var parts []string
	for name := range hard {
		usedQty := used[name]
		hardQty := hard[name]
		part := fmt.Sprintf("%s: %s/%s", name, usedQty.String(), hardQty.String())
		parts = append(parts, part)
	}
	// Sort for consistent output
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceAccountQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()
	logger := logf.FromContext(context.Background()).WithName("SetupWithManager")
	logger.Info("Starting ServiceAccountQuota controller")

	// Register FieldIndexer for ServiceAccountName
	if err := mgr.GetFieldIndexer().IndexField(ctx, &quotav1alpha1.ServiceAccountQuota{}, "spec.serviceAccountName", func(obj client.Object) []string {
		quota := obj.(*quotav1alpha1.ServiceAccountQuota)
		if quota.Spec.ServiceAccountName == "" {
			return []string{}
		}
		return []string{quota.Spec.ServiceAccountName}
	}); err != nil {
		return err
	}
	// Register FieldIndexer for Pod
	if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, fmt.Sprintf("metadata.annotations[%s]", annotations.ServiceAccountKey), func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if sa, ok := pod.Annotations[annotations.ServiceAccountKey]; ok {
			return []string{sa}
		}
		return nil
	}); err != nil {
		return err
	}

	// Build controller
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&quotav1alpha1.ServiceAccountQuota{}).
		Watches(&corev1.Pod{}, handler.Funcs{
			CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				pod, ok := e.Object.(*corev1.Pod)
				if !ok {
					return
				}

				saName, has := pod.Annotations[annotations.ServiceAccountKey]
				if !has {
					return
				}

				var quotaList quotav1alpha1.ServiceAccountQuotaList
				if err := r.List(ctx, &quotaList, client.MatchingFields{"spec.serviceAccountName": saName}); err != nil {
					return
				}

				for _, quota := range quotaList.Items {
					w.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{Name: quota.Name},
					})
				}
			},
			DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				pod, ok := e.Object.(*corev1.Pod)
				if !ok {
					return
				}

				saName, has := pod.Annotations[annotations.ServiceAccountKey]
				if !has {
					return
				}

				var quotaList quotav1alpha1.ServiceAccountQuotaList
				if err := r.List(ctx, &quotaList, client.MatchingFields{"spec.serviceAccountName": saName}); err != nil {
					return
				}

				for _, quota := range quotaList.Items {
					w.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{Name: quota.Name},
					})
				}
			},
			UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				_, ok1 := e.ObjectOld.(*corev1.Pod)
				newPod, ok2 := e.ObjectNew.(*corev1.Pod)
				if !ok1 || !ok2 {
					return
				}

				saName, has := newPod.Annotations[annotations.ServiceAccountKey]
				if !has {
					return
				}

				// Only handle pods that are not in the same phase
				// if oldPod.Status.Phase == newPod.Status.Phase {
				//	return
				// }

				var quotaList quotav1alpha1.ServiceAccountQuotaList
				if err := r.List(ctx, &quotaList, client.MatchingFields{"spec.serviceAccountName": saName}); err != nil {
					return
				}

				for _, quota := range quotaList.Items {
					w.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{Name: quota.Name},
					})
				}
			},
		}).
		Named("serviceaccountquota")

	if err := builder.Complete(r); err != nil {
		return err
	}

	// Trigger initial reconciliation after leader election
	go func() {
		logger.Info("Waiting for leader election")
		<-mgr.Elected()
		logger.Info("Leader elected")

		var quotaList quotav1alpha1.ServiceAccountQuotaList
		if err := mgr.GetClient().List(context.TODO(), &quotaList); err != nil {
			logger.Error(err, "Unable to list ServiceAccountResourceQuotas")
			return
		}
		for _, quota := range quotaList.Items {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: quota.Name,
				},
			}
			_, err := r.Reconcile(context.TODO(), req)
			if err != nil {
				logger.Error(err, "Unable to reconcile ServiceAccountResourceQuota")
			}
		}
		logger.Info("Initial reconcile for ServiceAccountResourceQuota completed")
	}()

	return nil
}
