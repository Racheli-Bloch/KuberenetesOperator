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
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	scalingv1 "github.com/example/scaling-operator/api/v1"
)

const natsJSZPath = "/jsz"

// Mock NATS response structures
type mockConsumerConfig struct {
	FilterSubject string `json:"filter_subject"`
}

type mockConsumerState struct {
	Pending int64 `json:"pending"`
}

type mockConsumer struct {
	Config mockConsumerConfig `json:"config"`
	State  mockConsumerState  `json:"state"`
}

type mockAccount struct {
	Consumers map[string]mockConsumer `json:"consumers"`
}

type mockNATSResponse struct {
	Accounts map[string]mockAccount `json:"accounts"`
}

var _ = Describe("ScalingRule Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a ScalingRule", func() {
		It("Should handle scaling up when pending messages exceed threshold", func() {
			ctx := context.Background()

			// Create a mock NATS server
			mockNATSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == natsJSZPath {
					response := mockNATSResponse{
						Accounts: map[string]mockAccount{
							"default": {
								Consumers: map[string]mockConsumer{
									"consumer1": {
										Config: mockConsumerConfig{
											FilterSubject: "orders.processing",
										},
										State: mockConsumerState{
											Pending: 75, // Above scale up threshold
										},
									},
								},
							},
						},
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						fmt.Printf("failed to encode response: %v\n", err)
					}
				} else {
					http.NotFound(w, r)
				}
			}))
			defer mockNATSServer.Close()

			// Create a test deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())

			// Create a ScalingRule
			scalingRule := &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scaling-rule",
					Namespace: "default",
				},
				Spec: scalingv1.ScalingRuleSpec{
					DeploymentName:      "test-deployment",
					Namespace:           "default",
					MinReplicas:         1,
					MaxReplicas:         5,
					NatsMonitoringURL:   mockNATSServer.URL,
					Subject:             "orders.processing",
					ScaleUpThreshold:    50,
					ScaleDownThreshold:  10,
					PollIntervalSeconds: 1, // Very short interval for testing
				},
			}
			Expect(k8sClient.Create(ctx, scalingRule)).Should(Succeed())

			// Wait for the deployment to be scaled up
			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-deployment",
					Namespace: "default",
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(5))) // Should scale up to max replicas (5)

			// Check that the ScalingRule status was updated
			Eventually(func() int32 {
				var updatedScalingRule scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-scaling-rule",
					Namespace: "default",
				}, &updatedScalingRule)
				if err != nil {
					return 0
				}
				return updatedScalingRule.Status.CurrentReplicas
			}, timeout, interval).Should(Equal(int32(5)))

			// Cleanup
			Expect(k8sClient.Delete(ctx, scalingRule)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, deployment)).Should(Succeed())
		})

		It("Should handle scaling down when pending messages are below threshold", func() {
			ctx := context.Background()

			// Create a mock NATS server with low pending messages
			mockNATSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == natsJSZPath {
					response := mockNATSResponse{
						Accounts: map[string]mockAccount{
							"default": {
								Consumers: map[string]mockConsumer{
									"consumer1": {
										Config: mockConsumerConfig{
											FilterSubject: "orders.processing",
										},
										State: mockConsumerState{
											Pending: 5, // Below scale down threshold
										},
									},
								},
							},
						},
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						fmt.Printf("failed to encode response: %v\n", err)
					}
				} else {
					http.NotFound(w, r)
				}
			}))
			defer mockNATSServer.Close()

			// Create a test deployment with 3 replicas
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment-2",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test2"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test2"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())

			// Create a ScalingRule
			scalingRule := &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scaling-rule-2",
					Namespace: "default",
				},
				Spec: scalingv1.ScalingRuleSpec{
					DeploymentName:      "test-deployment-2",
					Namespace:           "default",
					MinReplicas:         1,
					MaxReplicas:         5,
					NatsMonitoringURL:   mockNATSServer.URL,
					Subject:             "orders.processing",
					ScaleUpThreshold:    50,
					ScaleDownThreshold:  10,
					PollIntervalSeconds: 1, // Very short interval for testing
				},
			}
			Expect(k8sClient.Create(ctx, scalingRule)).Should(Succeed())

			// Wait for the deployment to be scaled down
			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-deployment-2",
					Namespace: "default",
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1))) // Should scale down to min replicas (1)

			// Cleanup
			Expect(k8sClient.Delete(ctx, scalingRule)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, deployment)).Should(Succeed())
		})

		It("Should respect min and max replica limits", func() {
			ctx := context.Background()

			// Create a mock NATS server with very high pending messages
			mockNATSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == natsJSZPath {
					response := mockNATSResponse{
						Accounts: map[string]mockAccount{
							"default": {
								Consumers: map[string]mockConsumer{
									"consumer1": {
										Config: mockConsumerConfig{
											FilterSubject: "orders.processing",
										},
										State: mockConsumerState{
											Pending: 1000, // Very high, should trigger multiple scale ups
										},
									},
								},
							},
						},
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						fmt.Printf("failed to encode response: %v\n", err)
					}
				} else {
					http.NotFound(w, r)
				}
			}))
			defer mockNATSServer.Close()

			// Create a test deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment-3",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test3"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test3"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())

			// Create a ScalingRule with max 3 replicas
			scalingRule := &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scaling-rule-3",
					Namespace: "default",
				},
				Spec: scalingv1.ScalingRuleSpec{
					DeploymentName:      "test-deployment-3",
					Namespace:           "default",
					MinReplicas:         1,
					MaxReplicas:         3, // Max 3 replicas
					NatsMonitoringURL:   mockNATSServer.URL,
					Subject:             "orders.processing",
					ScaleUpThreshold:    50,
					ScaleDownThreshold:  10,
					PollIntervalSeconds: 1, // Very short interval for testing
				},
			}
			Expect(k8sClient.Create(ctx, scalingRule)).Should(Succeed())

			// Wait for the deployment to reach max replicas
			Eventually(func() int32 {
				var updatedDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-deployment-3",
					Namespace: "default",
				}, &updatedDeployment)
				if err != nil {
					return 0
				}
				return *updatedDeployment.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3))) // Should not exceed max replicas

			// Cleanup
			Expect(k8sClient.Delete(ctx, scalingRule)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, deployment)).Should(Succeed())
		})
	})
})

// Helper function to create int32 pointers
func int32Ptr(i int32) *int32 {
	return &i
}
