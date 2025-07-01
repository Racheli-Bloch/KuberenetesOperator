# Kubernetes NATS Scaling Operator

A production-ready Kubernetes operator written in Go that dynamically scales Deployments based on message traffic observed in NATS queues using monitoring metrics.

## Features

- **Dynamic Scaling**: Automatically scales Deployments up/down based on NATS queue length
- **NATS Integration**: Uses NATS monitoring endpoint (`/jsz`) to determine pending messages
- **Multiple ScalingRules**: Support for multiple concurrent ScalingRule CRDs
- **Observability**: HTTP endpoint exposing recent scaling activity with nice UI
- **Production Ready**: Comprehensive error handling, logging, and status updates
- **Extensible**: Clean architecture with separation of concerns

## Architecture

The operator consists of:

1. **Custom Resource Definition (CRD)**: `ScalingRule` defines scaling configuration
2. **Controller**: Watches ScalingRules and manages Deployment scaling
3. **HTTP Server**: Exposes scaling activity and health endpoints
4. **NATS Integration**: Queries NATS monitoring for queue metrics

## Prerequisites

- Go 1.21+
- Kubernetes cluster (kind, minikube, or production)
- NATS server with JetStream enabled
- kubectl configured to access your cluster

## Installation

### 1. Clone the Repository

```bash
git clone [KuberenetesOperator repo URL](https://github.com/Racheli-Bloch/KuberenetesOperator.git)
cd scaling-operator
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Install CRDs and Deploy the Operator

```bash
# Install CRDs
make install

# Deploy the operator
make deploy
```

### 4. Verify Installation

```bash
# Check if the operator is running
kubectl get pods -n scaling-operator-system

# Check if CRD is installed
kubectl get crd scalingrules.scaling.example.com
```

## Usage

### 1. Create a ScalingRule

Create a `ScalingRule` custom resource to define scaling behavior:

```yaml
apiVersion: scaling.example.com/v1
kind: ScalingRule
metadata:
  name: my-app-scaling
  namespace: default
spec:
  # Target deployment
  deploymentName: "my-app"
  namespace: "default"
  
  # Scaling limits
  minReplicas: 1
  maxReplicas: 10
  
  # NATS configuration
  natsMonitoringURL: "http://nats:8222"
  subject: "orders.processing"
  
  # Scaling thresholds
  scaleUpThreshold: 50    # Scale up when pending messages > 50
  scaleDownThreshold: 10  # Scale down when pending messages < 10
  
  # Polling interval
  pollIntervalSeconds: 30
```

Apply the ScalingRule:

```bash
kubectl apply -f config/samples/scaling_v1_scalingrule.yaml
```

### 2. Monitor Scaling Activity

The operator exposes an HTTP endpoint for viewing scaling activity:

```bash
# Port forward to access the activity server
kubectl port-forward -n scaling-operator-system deployment/scaling-operator-controller-manager 8082:8082

# View scaling activity in browser
open http://localhost:8082

# Or get JSON data
curl http://localhost:8082/scaling-activity
```

### 3. Check ScalingRule Status

```bash
# View ScalingRule details
kubectl get scalingrules
kubectl describe scalingrule my-app-scaling

# Check status
kubectl get scalingrule my-app-scaling -o yaml
```

## Configuration

### ScalingRule Specification

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `deploymentName` | string | Yes | Name of the Deployment to scale |
| `namespace` | string | Yes | Namespace where the Deployment is located |
| `minReplicas` | int32 | Yes | Minimum number of replicas (≥1) |
| `maxReplicas` | int32 | Yes | Maximum number of replicas (≥1) |
| `natsMonitoringURL` | string | Yes | NATS monitoring endpoint URL |
| `subject` | string | Yes | NATS queue subject to monitor |
| `scaleUpThreshold` | int32 | Yes | Messages threshold to trigger scale up |
| `scaleDownThreshold` | int32 | Yes | Messages threshold to trigger scale down |
| `pollIntervalSeconds` | int32 | Yes | Polling interval in seconds |

### Scaling Logic

- **Scale Up**: When pending messages > `scaleUpThreshold` AND current replicas < `maxReplicas`
- **Scale Down**: When pending messages < `scaleDownThreshold` AND current replicas > `minReplicas`
- **No Action**: When conditions are not met or already at limits

## Testing

### Run Unit Tests

```bash
make test
```

### Run Integration Tests with envtest

```bash
# Run tests in Docker
make test-docker

# Or run locally
make test-integration
```

### Manual Testing

1. **Deploy a test application**:
```bash
kubectl create deployment test-app --image=nginx:latest --replicas=2
```

2. **Create a ScalingRule**:
```bash
kubectl apply -f config/samples/scaling_v1_scalingrule.yaml
```

3. **Simulate NATS traffic** (you'll need a NATS server running)

4. **Monitor scaling**:
```bash
kubectl get scalingrules
kubectl get deployments
```

## Development

### Local Development

```bash
# Run the operator locally
make run

# Generate code after API changes
make generate

# Build the operator
make build

# Build Docker image
make docker-build
```

### Project Structure

```
├── api/v1/                    # API definitions
│   └── scalingrule_types.go   # ScalingRule CRD
├── cmd/main.go               # Main application entry point
├── internal/controller/      # Controller logic
│   └── scalingrule_controller.go
├── pkg/server/              # HTTP server for activity
│   └── server.go
├── config/                  # Kubernetes manifests
│   ├── crd/                # CRD definitions
│   ├── rbac/               # RBAC rules
│   └── samples/            # Sample configurations
└── test/                   # Test files
```

## Monitoring and Observability

### Logs

The operator provides structured logging for all scaling actions:

```bash
kubectl logs -n scaling-operator-system deployment/scaling-operator-controller-manager
```

### Metrics

Prometheus metrics are available at the metrics endpoint (if enabled).

### HTTP Endpoints

- `/` - Web UI for scaling activity
- `/scaling-activity` - JSON API for scaling history
- `/health` - Health check endpoint

## Troubleshooting

### Common Issues

1. **Deployment not found**:
   - Verify the deployment exists in the specified namespace
   - Check the `deploymentName` and `namespace` fields

2. **NATS connection issues**:
   - Ensure NATS server is running and accessible
   - Verify the `natsMonitoringURL` is correct
   - Check network connectivity

3. **No scaling occurring**:
   - Check NATS queue has messages for the specified subject
   - Verify thresholds are appropriate
   - Check operator logs for errors

### Debug Mode

Enable debug logging:

```bash
kubectl set env -n scaling-operator-system deployment/scaling-operator-controller-manager LOG_LEVEL=debug
```

## Deployment Options

### Using Kustomize (Recommended)

The operator comes with Kustomize configurations for easy deployment:

```bash
# Deploy with default configuration
kubectl apply -k config/default

# Deploy with production overlay (2 replicas, production settings)
kubectl apply -k config/overlays/production

# Or use make targets
make install    # Install CRDs
make deploy     # Deploy operator
```

### Using Helm (Alternative)

A Helm chart is available for advanced customization:

```bash
# Install with default values
helm install scaling-operator ./helm/scaling-operator

# Install with custom values
helm install scaling-operator ./helm/scaling-operator \
  --set replicaCount=2 \
  --set operator.logLevel=debug \
  --set operator.activityServer.port=8082

# Upgrade existing installation
helm upgrade scaling-operator ./helm/scaling-operator

# Uninstall
helm uninstall scaling-operator
```

### Custom Values for Helm

Create a `values-custom.yaml` file:

```yaml
replicaCount: 2
image:
  repository: your-registry/scaling-operator
  tag: "v1.0.0"

operator:
  logLevel: "info"
  metrics:
    enabled: true
    port: 8080
  activityServer:
    enabled: true
    port: 8082
    maxActions: 200

resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

Then install with:
```bash
helm install scaling-operator ./helm/scaling-operator -f values-custom.yaml
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Apache 2.0 License - see LICENSE file for details.

## Support

For issues and questions:
- Create an issue in the repository
- Check the troubleshooting section
- Review the logs for error details

