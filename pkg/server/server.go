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

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ScalingAction represents a single scaling action
type ScalingAction struct {
	Timestamp       time.Time `json:"timestamp"`
	Deployment      string    `json:"deployment"`
	Namespace       string    `json:"namespace"`
	FromReplicas    int32     `json:"fromReplicas"`
	ToReplicas      int32     `json:"toReplicas"`
	Reason          string    `json:"reason"`
	PendingMessages int32     `json:"pendingMessages"`
}

// ScalingActivityServer provides HTTP endpoints for scaling activity
type ScalingActivityServer struct {
	actions []ScalingAction
	mutex   sync.RWMutex
	maxSize int
	port    int
}

// NewScalingActivityServer creates a new server instance
func NewScalingActivityServer(port int, maxActions int) *ScalingActivityServer {
	return &ScalingActivityServer{
		actions: make([]ScalingAction, 0, maxActions),
		maxSize: maxActions,
		port:    port,
	}
}

// RecordScalingAction adds a new scaling action to the history
func (s *ScalingActivityServer) RecordScalingAction(action ScalingAction) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Add timestamp if not set
	if action.Timestamp.IsZero() {
		action.Timestamp = time.Now()
	}

	// Add to the beginning (most recent first)
	s.actions = append([]ScalingAction{action}, s.actions...)

	// Keep only the most recent actions
	if len(s.actions) > s.maxSize {
		s.actions = s.actions[:s.maxSize]
	}
}

// GetScalingActivity returns the recent scaling activity
func (s *ScalingActivityServer) GetScalingActivity() []ScalingAction {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]ScalingAction, len(s.actions))
	copy(result, s.actions)
	return result
}

// Start starts the HTTP server
func (s *ScalingActivityServer) Start() error {
	http.HandleFunc("/scaling-activity", s.handleScalingActivity)
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/", s.handleRoot)

	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("Starting scaling activity server on port %d\n", s.port)
	return http.ListenAndServe(addr, nil)
}

// handleScalingActivity handles the /scaling-activity endpoint
func (s *ScalingActivityServer) handleScalingActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	actions := s.GetScalingActivity()
	response := map[string]interface{}{
		"timestamp": time.Now(),
		"actions":   actions,
		"count":     len(actions),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Optionally log the error
	}
}

// handleHealth handles the /health endpoint
func (s *ScalingActivityServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "scaling-operator",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Optionally log the error
	}
}

// handleRoot handles the root endpoint
func (s *ScalingActivityServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Scaling Operator Activity</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .action { border: 1px solid #ddd; margin: 10px 0; padding: 15px; border-radius: 5px; }
        .scale-up { border-left: 5px solid #4CAF50; }
        .scale-down { border-left: 5px solid #f44336; }
        .timestamp { color: #666; font-size: 0.9em; }
        .reason { font-weight: bold; margin: 10px 0; }
        .details { color: #333; }
    </style>
</head>
<body>
    <h1>Scaling Operator Activity</h1>
    <p><a href="/scaling-activity">View JSON API</a> | <a href="/health">Health Check</a></p>
    <div id="actions"></div>
    
    <script>
        function loadActions() {
            fetch('/scaling-activity')
                .then(response => response.json())
                .then(data => {
                    const container = document.getElementById('actions');
                    container.innerHTML = '';
                    
                    if (data.actions.length === 0) {
                        container.innerHTML = '<p>No scaling actions recorded yet.</p>';
                        return;
                    }
                    
                    data.actions.forEach(action => {
                        const div = document.createElement('div');
                        div.className = 'action ' +
                            (action.toReplicas > action.fromReplicas ? 'scale-up' : 'scale-down');
                        
                        const timestamp = new Date(action.timestamp).toLocaleString();
                        div.innerHTML = '<div class="timestamp">' + timestamp + '</div>' +
                            '<div class="reason">' + action.reason + '</div>' +
                            '<div class="details">' +
                            '<strong>' + action.deployment + '</strong> in <strong>' +
                            action.namespace + '</strong><br>' +
                            'Replicas: ' + action.fromReplicas + ' \u2192 ' + action.toReplicas + '<br>' +
                            'Pending Messages: ' + action.pendingMessages +
                            '</div>';
                        container.appendChild(div);
                    });
                })
                .catch(error => {
                    console.error('Error loading actions:', error);
                    document.getElementById('actions').innerHTML = '<p>Error loading scaling actions.</p>';
                });
        }
        
        // Load actions immediately and refresh every 30 seconds
        loadActions();
        setInterval(loadActions, 30000);
    </script>
</body>
</html>`
	if _, err := w.Write([]byte(html)); err != nil {
		// Optionally log the error
	}
}
