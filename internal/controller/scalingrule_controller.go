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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	scalingv1 "github.com/example/scaling-operator/api/v1"
	"github.com/example/scaling-operator/pkg/server"
)

// ScalingRuleReconciler reconciles a ScalingRule object
type ScalingRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Server *server.ScalingActivityServer
}

// NATSJetStreamStats represents the response from NATS /jsz endpoint
type NATSJetStreamStats struct {
	Accounts map[string]struct {
		Streams map[string]struct {
			State struct {
				Messages int64 `json:"messages"`
				Bytes    int64 `json:"bytes"`
			} `json:"state"`
			Config struct {
				Subjects []string `json:"subjects"`
			} `json:"config"`
		} `json:"streams"`
		Consumers map[string]struct {
			Config struct {
				FilterSubject string `json:"filter_subject"`
			} `json:"config"`
			State struct {
				Pending     int64 `json:"pending"`
				Delivered   int64 `json:"delivered"`
				Redelivered int64 `json:"redelivered"`
			} `json:"state"`
		} `json:"consumers"`
	} `json:"accounts"`
}

// +kubebuilder:rbac:groups=scaling.example.com,resources=scalingrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scaling.example.com,resources=scalingrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scaling.example.com,resources=scalingrules/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ScalingRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Starting reconciliation", "scalingRule", req.NamespacedName)

	// Fetch the ScalingRule
	scalingRule := &scalingv1.ScalingRule{}
	err := r.Get(ctx, req.NamespacedName, scalingRule)
	if err != nil {
		if errors.IsNotFound(err) {
			// ScalingRule not found, return without error
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ScalingRule")
		return ctrl.Result{}, err
	}

	// Get pending messages from NATS
	pendingMessages, err := r.getPendingMessages(ctx, scalingRule.Spec.NatsMonitoringURL, scalingRule.Spec.Subject)
	if err != nil {
		log.Error(err, "Failed to get pending messages from NATS", "url", scalingRule.Spec.NatsMonitoringURL, "subject", scalingRule.Spec.Subject)
		r.updateStatus(ctx, scalingRule, 0, 0, "Failed to query NATS", err)
		return ctrl.Result{RequeueAfter: time.Duration(scalingRule.Spec.PollIntervalSeconds) * time.Second}, nil
	}

	// Get the target deployment
	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: scalingRule.Spec.Namespace,
		Name:      scalingRule.Spec.DeploymentName,
	}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Target deployment not found", "deployment", scalingRule.Spec.DeploymentName, "namespace", scalingRule.Spec.Namespace)
			r.updateStatus(ctx, scalingRule, 0, pendingMessages, "Deployment not found", err)
			return ctrl.Result{RequeueAfter: time.Duration(scalingRule.Spec.PollIntervalSeconds) * time.Second}, nil
		}
		log.Error(err, "Failed to get deployment")
		return ctrl.Result{}, err
	}

	currentReplicas := *deployment.Spec.Replicas
	desiredReplicas := currentReplicas
	scalingReason := "No scaling needed"

	// Determine if scaling is needed
	if pendingMessages > scalingRule.Spec.ScaleUpThreshold && currentReplicas < scalingRule.Spec.MaxReplicas {
		desiredReplicas = currentReplicas + 1
		if desiredReplicas > scalingRule.Spec.MaxReplicas {
			desiredReplicas = scalingRule.Spec.MaxReplicas
		}
		scalingReason = fmt.Sprintf("Scale up: pending messages (%d) > threshold (%d)", pendingMessages, scalingRule.Spec.ScaleUpThreshold)
	} else if pendingMessages < scalingRule.Spec.ScaleDownThreshold && currentReplicas > scalingRule.Spec.MinReplicas {
		desiredReplicas = currentReplicas - 1
		if desiredReplicas < scalingRule.Spec.MinReplicas {
			desiredReplicas = scalingRule.Spec.MinReplicas
		}
		scalingReason = fmt.Sprintf("Scale down: pending messages (%d) < threshold (%d)", pendingMessages, scalingRule.Spec.ScaleDownThreshold)
	}

	// Perform scaling if needed
	if desiredReplicas != currentReplicas {
		log.Info("Scaling deployment",
			"deployment", scalingRule.Spec.DeploymentName,
			"namespace", scalingRule.Spec.Namespace,
			"currentReplicas", currentReplicas,
			"desiredReplicas", desiredReplicas,
			"reason", scalingReason,
			"pendingMessages", pendingMessages)

		// Update deployment replicas
		deployment.Spec.Replicas = &desiredReplicas
		err = r.Update(ctx, deployment)
		if err != nil {
			log.Error(err, "Failed to update deployment replicas")
			r.updateStatus(ctx, scalingRule, currentReplicas, pendingMessages, "Failed to scale deployment", err)
			return ctrl.Result{}, err
		}

		// Log the scaling action with timestamp
		log.Info("Deployment scaled successfully",
			"deployment", scalingRule.Spec.DeploymentName,
			"namespace", scalingRule.Spec.Namespace,
			"from", currentReplicas,
			"to", desiredReplicas,
			"reason", scalingReason,
			"timestamp", time.Now().Format(time.RFC3339))

		// Record scaling action in HTTP server
		if r.Server != nil {
			action := server.ScalingAction{
				Timestamp:       time.Now(),
				Deployment:      scalingRule.Spec.DeploymentName,
				Namespace:       scalingRule.Spec.Namespace,
				FromReplicas:    currentReplicas,
				ToReplicas:      desiredReplicas,
				Reason:          scalingReason,
				PendingMessages: pendingMessages,
			}
			r.Server.RecordScalingAction(action)
		}
	}

	// Update status
	r.updateStatus(ctx, scalingRule, desiredReplicas, pendingMessages, scalingReason, nil)

	// Requeue after the poll interval
	return ctrl.Result{RequeueAfter: time.Duration(scalingRule.Spec.PollIntervalSeconds) * time.Second}, nil
}

// getPendingMessages queries the NATS monitoring endpoint to get pending messages for a subject
func (r *ScalingRuleReconciler) getPendingMessages(ctx context.Context, monitoringURL, subject string) (int32, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/jsz", monitoringURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to query NATS monitoring: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("NATS monitoring returned status %d", resp.StatusCode)
	}

	var stats NATSJetStreamStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return 0, fmt.Errorf("failed to decode NATS response: %w", err)
	}

	// Find pending messages for the specified subject
	var totalPending int64
	for _, account := range stats.Accounts {
		for _, consumer := range account.Consumers {
			if consumer.Config.FilterSubject == subject {
				totalPending += consumer.State.Pending
			}
		}
	}

	return int32(totalPending), nil
}

// updateStatus updates the ScalingRule status
func (r *ScalingRuleReconciler) updateStatus(ctx context.Context, scalingRule *scalingv1.ScalingRule, currentReplicas, pendingMessages int32, reason string, err error) {
	log := logf.FromContext(ctx)

	// Update status fields
	scalingRule.Status.CurrentReplicas = currentReplicas
	scalingRule.Status.PendingMessages = pendingMessages

	if err == nil && reason != "No scaling needed" {
		now := metav1.Now()
		scalingRule.Status.LastScalingAction = &now
		scalingRule.Status.LastScalingReason = reason
	}

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ScalingRule is ready. Current replicas: %d, Pending messages: %d", currentReplicas, pendingMessages),
		LastTransitionTime: metav1.Now(),
	}

	if err != nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "Error"
		condition.Message = fmt.Sprintf("Error: %v", err)
	}

	// Update or add the condition
	conditionUpdated := false
	for i, existingCondition := range scalingRule.Status.Conditions {
		if existingCondition.Type == condition.Type {
			scalingRule.Status.Conditions[i] = condition
			conditionUpdated = true
			break
		}
	}
	if !conditionUpdated {
		scalingRule.Status.Conditions = append(scalingRule.Status.Conditions, condition)
	}

	// Update the status
	if err := r.Status().Update(ctx, scalingRule); err != nil {
		log.Error(err, "Failed to update ScalingRule status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScalingRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scalingv1.ScalingRule{}).
		Named("scalingrule").
		Complete(r)
}
