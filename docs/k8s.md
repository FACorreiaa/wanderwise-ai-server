# Kubernetes Deployment Guide for Go AI POI Monorepo

## Overview

This guide explains how to deploy the Go AI POI application on Kubernetes. The current architecture is a gRPC-enabled monorepo with a single Go module, providing a consolidated deployment approach with integrated observability.

## Architecture

### Monorepo Application Structure
- **Single Go Application** with gRPC and HTTP endpoints
- **HTTP Port**: 8080 (metrics, health checks, service registry endpoints)
- **gRPC Port**: 9000 (internal service communication)
- **Metrics Port**: Prometheus metrics on `/metrics`
- **Health Endpoints**: `/health`, `/services`, `/services/stats`, `/services/healthy`

### Core Features
- **Authentication & Authorization** - JWT-based auth system
- **POI Management** - Points of interest with AI-powered recommendations
- **Chat Service** - AI-powered chat interactions
- **User Management** - Profile, preferences, and lists
- **Reviews & Ratings** - Community-driven content
- **Analytics** - Usage statistics and insights
- **AI Integration** - OpenAI GPT integration for recommendations

### Infrastructure Components
- **PostgreSQL** with PostGIS and pgvector extensions
- **Redis** for caching and session management

### Observability Stack
- **Prometheus** - Metrics collection and storage
- **Grafana** - Visualization and dashboards
- **Tempo** - Distributed tracing
- **Loki** - Log aggregation
- **Promtail** - Log collection agent

## Kubernetes Deployment Strategy

### 1. Namespace Setup

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: go-ai-poi
---
apiVersion: v1
kind: Namespace
metadata:
  name: observability
```

### 2. ConfigMaps and Secrets

#### Database Configuration
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
  namespace: go-ai-poi
type: Opaque
data:
  POSTGRES_USER: bG9jaQ==  # base64: loci
  POSTGRES_PASSWORD: bG9jaTE2NHJyaQ==  # base64: loci123
  POSTGRES_DB: bG9jaQ==  # base64: loci
---
apiVersion: v1
kind: Secret
metadata:
  name: go-ai-poi-secrets
  namespace: go-ai-poi
type: Opaque
data:
  JWT_SECRET: eW91ci1qd3Qtc2VjcmV0LWtleS1oZXJl  # base64: your-jwt-secret-key-here
  OPENAI_API_KEY: eW91ci1vcGVuYWktYXBpLWtleS1oZXJl  # base64: your-openai-api-key-here
```

#### Application Configuration
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: go-ai-poi-config
  namespace: go-ai-poi
data:
  POSTGRES_HOST: postgres-service
  POSTGRES_PORT: "5432"
  REDIS_HOST: redis-service
  REDIS_PORT: "6379"
  HTTP_PORT: "8080"
  GRPC_PORT: "9000"
  LOG_LEVEL: "info"
  OPENAI_MODEL: "gpt-4"
```

### 3. Persistent Volumes

#### PostgreSQL Storage
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: go-ai-poi
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
  storageClassName: fast-ssd  # Adjust based on your storage class
```

#### Observability Storage
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: prometheus-pvc
  namespace: observability
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: grafana-pvc
  namespace: observability
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
```

### 4. Database Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: go-ai-poi
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgis/postgis:17-3.5
        env:
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_USER
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_PASSWORD
        - name: POSTGRES_DB
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_DB
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        - name: init-scripts
          mountPath: /docker-entrypoint-initdb.d
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-pvc
      - name: init-scripts
        configMap:
          name: postgres-init-scripts
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: go-ai-poi
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
  type: ClusterIP
```

### 5. Redis Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: go-ai-poi
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        command: ["redis-server", "--appendonly", "yes"]
        ports:
        - containerPort: 6379
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "200m"
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: go-ai-poi
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
  type: ClusterIP
```

### 6. Go AI POI Application Deployment

Single application deployment with both HTTP and gRPC endpoints:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-ai-poi-app
  namespace: go-ai-poi
  labels:
    app: go-ai-poi-app
    version: v1.0.0
spec:
  replicas: 3  # Adjust based on load requirements
  selector:
    matchLabels:
      app: go-ai-poi-app
  template:
    metadata:
      labels:
        app: go-ai-poi-app
        version: v1.0.0
    spec:
      containers:
      - name: go-ai-poi-app
        image: your-registry/go-ai-poi:latest
        env:
        - name: SERVICE_NAME
          value: "go-ai-poi-app"
        envFrom:
        - configMapRef:
            name: go-ai-poi-config
        - secretRef:
            name: postgres-secret
        - secretRef:
            name: go-ai-poi-secrets  # For JWT secret, OpenAI API key, etc.
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9000
          name: grpc
        - containerPort: 6060
          name: pprof
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "512Mi"
            cpu: "200m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
---
apiVersion: v1
kind: Service
metadata:
  name: go-ai-poi-service
  namespace: go-ai-poi
  labels:
    app: go-ai-poi-app
spec:
  selector:
    app: go-ai-poi-app
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9000
    targetPort: 9000
    name: grpc
    protocol: TCP
  type: ClusterIP
```

### 7. Observability Stack

#### Prometheus
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: observability
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        args:
          - '--config.file=/etc/prometheus/prometheus.yml'
          - '--storage.tsdb.path=/prometheus'
          - '--web.console.libraries=/etc/prometheus/console_libraries'
          - '--web.console.templates=/etc/prometheus/consoles'
          - '--web.enable-lifecycle'
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: prometheus-config
          mountPath: /etc/prometheus
        - name: prometheus-storage
          mountPath: /prometheus
        resources:
          requests:
            memory: "512Mi"
            cpu: "200m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
      volumes:
      - name: prometheus-config
        configMap:
          name: prometheus-config
      - name: prometheus-storage
        persistentVolumeClaim:
          claimName: prometheus-pvc
```

#### Grafana
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: observability
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      containers:
      - name: grafana
        image: grafana/grafana:latest
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          value: "admin"
        - name: GF_USERS_ALLOW_SIGN_UP
          value: "false"
        ports:
        - containerPort: 3000
        volumeMounts:
        - name: grafana-storage
          mountPath: /var/lib/grafana
        - name: grafana-datasources
          mountPath: /etc/grafana/provisioning/datasources
        - name: grafana-dashboards
          mountPath: /etc/grafana/provisioning/dashboards
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "200m"
      volumes:
      - name: grafana-storage
        persistentVolumeClaim:
          claimName: grafana-pvc
      - name: grafana-datasources
        configMap:
          name: grafana-datasources
      - name: grafana-dashboards
        configMap:
          name: grafana-dashboards
```

### 8. Service Mesh Considerations

For production deployments, consider implementing a service mesh like Istio for:

- **mTLS encryption** between services
- **Traffic management** and load balancing
- **Circuit breaking** and retry policies
- **Observability** enhancements
- **Security policies** and authorization

#### Istio Configuration Example
```yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: go-ai-poi
spec:
  mtls:
    mode: STRICT
---
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: auth-service
  namespace: go-ai-poi
spec:
  hosts:
  - auth-service
  http:
  - route:
    - destination:
        host: auth-service
        port:
          number: 8001
    fault:
      delay:
        percentage:
          value: 0.1
        fixedDelay: 5s
    retries:
      attempts: 3
      perTryTimeout: 2s
```

### 9. Ingress Configuration

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: go-ai-poi-ingress
  namespace: go-ai-poi
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/use-regex: "true"
    nginx.ingress.kubernetes.io/proxy-connect-timeout: "600"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "600"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - api.your-domain.com
    secretName: go-ai-poi-tls
  rules:
  - host: api.your-domain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: go-ai-poi-service
            port:
              number: 8080
```

### 10. Horizontal Pod Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: go-ai-poi-hpa
  namespace: go-ai-poi
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: go-ai-poi-app
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

### 11. Resource Recommendations

#### Production Resource Limits

| Component | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|-------------|-----------|----------------|--------------|
| Go AI POI App | 200m | 1000m | 512Mi | 1Gi |
| PostgreSQL | 500m | 2000m | 1Gi | 4Gi |
| Redis | 100m | 200m | 256Mi | 512Mi |
| Prometheus | 200m | 1000m | 512Mi | 2Gi |
| Grafana | 100m | 200m | 256Mi | 512Mi |
| Loki | 100m | 200m | 256Mi | 512Mi |
| Tempo | 100m | 200m | 256Mi | 512Mi |

### 12. Security Best Practices

1. **Network Policies**: Restrict inter-pod communication
2. **RBAC**: Implement proper role-based access control
3. **Pod Security Standards**: Use restricted security contexts
4. **Secrets Management**: Use external secret managers (e.g., Vault)
5. **Image Security**: Scan images for vulnerabilities
6. **Service Accounts**: Use minimal privilege service accounts

### 13. Monitoring and Alerting

#### Key Metrics to Monitor
- **Service Health**: HTTP/gRPC response times and error rates
- **Resource Usage**: CPU, memory, and network utilization
- **Database Performance**: Connection pool usage, query performance
- **Business Metrics**: User registrations, API usage patterns

#### Alert Rules
```yaml
groups:
- name: go-ai-poi-alerts
  rules:
  - alert: HighErrorRate
    expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: High error rate detected
  
  - alert: HighMemoryUsage
    expr: container_memory_usage_bytes / container_spec_memory_limit_bytes > 0.9
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: Container memory usage is above 90%
```

### 14. Deployment Commands

```bash
# Apply namespace
kubectl apply -f namespaces.yaml

# Apply secrets and configmaps
kubectl apply -f secrets.yaml
kubectl apply -f configmaps.yaml

# Apply persistent volumes
kubectl apply -f pvcs.yaml

# Deploy infrastructure
kubectl apply -f postgres.yaml
kubectl apply -f redis.yaml

# Deploy main application
kubectl apply -f go-ai-poi-app.yaml

# Deploy observability stack
kubectl apply -f prometheus.yaml
kubectl apply -f grafana.yaml
kubectl apply -f tempo.yaml
kubectl apply -f loki.yaml

# Apply ingress and HPA
kubectl apply -f ingress.yaml
kubectl apply -f hpa.yaml

# Verify deployment
kubectl get pods -n go-ai-poi
kubectl get services -n go-ai-poi
kubectl get ingress -n go-ai-poi
```

### 15. Infrastructure as Code with Terraform

#### Cloud Provider Setup (AWS Example)

```hcl
# terraform/main.tf
terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.11"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# EKS Cluster
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"

  cluster_name    = "go-ai-poi-cluster"
  cluster_version = "1.28"

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  # EKS Managed Node Groups
  eks_managed_node_groups = {
    main = {
      min_size     = 2
      max_size     = 10
      desired_size = 3

      instance_types = ["t3.medium", "t3.large"]
      capacity_type  = "ON_DEMAND"

      k8s_labels = {
        Environment = var.environment
        Application = "go-ai-poi"
      }
    }

    spot = {
      min_size     = 0
      max_size     = 5
      desired_size = 2

      instance_types = ["t3.medium", "t3.large", "t3.xlarge"]
      capacity_type  = "SPOT"

      k8s_labels = {
        Environment = var.environment
        Application = "go-ai-poi"
        NodeType    = "spot"
      }

      taints = {
        spot = {
          key    = "spot"
          value  = "true"
          effect = "NO_SCHEDULE"
        }
      }
    }
  }

  # Cluster access entry
  access_entries = {
    admin = {
      kubernetes_groups = []
      principal_arn     = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/AdminRole"

      policy_associations = {
        admin = {
          policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
          access_scope = {
            type = "cluster"
          }
        }
      }
    }
  }

  tags = {
    Environment = var.environment
    Project     = "go-ai-poi"
  }
}

# VPC
module "vpc" {
  source = "terraform-aws-modules/vpc/aws"

  name = "go-ai-poi-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["${var.aws_region}a", "${var.aws_region}b", "${var.aws_region}c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway = true
  enable_vpn_gateway = true

  tags = {
    "kubernetes.io/cluster/go-ai-poi-cluster" = "shared"
  }

  public_subnet_tags = {
    "kubernetes.io/role/elb" = "1"
  }

  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = "1"
  }
}

# RDS PostgreSQL Instance
resource "aws_db_instance" "postgres" {
  identifier             = "go-ai-poi-postgres"
  engine                 = "postgres"
  engine_version         = "15.4"
  instance_class         = "db.t3.micro"
  allocated_storage      = 20
  max_allocated_storage  = 100
  storage_encrypted      = true

  db_name  = var.postgres_db_name
  username = var.postgres_username
  password = var.postgres_password

  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.postgres.name

  backup_retention_period = 7
  backup_window          = "03:00-04:00"
  maintenance_window     = "sun:04:00-sun:05:00"

  skip_final_snapshot = true
  deletion_protection = false

  tags = {
    Name        = "go-ai-poi-postgres"
    Environment = var.environment
  }
}

# ElastiCache Redis Cluster
resource "aws_elasticache_subnet_group" "redis" {
  name       = "go-ai-poi-redis-subnet-group"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id         = "go-ai-poi-redis"
  description                  = "Redis cluster for go-ai-poi"
  
  node_type                    = "cache.t3.micro"
  port                         = 6379
  parameter_group_name         = "default.redis7"
  
  num_cache_clusters           = 2
  
  subnet_group_name            = aws_elasticache_subnet_group.redis.name
  security_group_ids           = [aws_security_group.redis.id]
  
  at_rest_encryption_enabled   = true
  transit_encryption_enabled   = true
  
  tags = {
    Name        = "go-ai-poi-redis"
    Environment = var.environment
  }
}

# Security Groups
resource "aws_security_group" "rds" {
  name_prefix = "go-ai-poi-rds-"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "go-ai-poi-rds-sg"
  }
}

resource "aws_security_group" "redis" {
  name_prefix = "go-ai-poi-redis-"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "go-ai-poi-redis-sg"
  }
}

# DB Subnet Group
resource "aws_db_subnet_group" "postgres" {
  name       = "go-ai-poi-postgres-subnet-group"
  subnet_ids = module.vpc.private_subnets

  tags = {
    Name = "go-ai-poi-postgres-subnet-group"
  }
}
```

#### Variables and Outputs

```hcl
# terraform/variables.tf
variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "production"
}

variable "postgres_db_name" {
  description = "PostgreSQL database name"
  type        = string
  default     = "loci"
}

variable "postgres_username" {
  description = "PostgreSQL username"
  type        = string
  default     = "loci"
}

variable "postgres_password" {
  description = "PostgreSQL password"
  type        = string
  sensitive   = true
}

# terraform/outputs.tf
output "cluster_endpoint" {
  description = "Endpoint for EKS control plane"
  value       = module.eks.cluster_endpoint
}

output "cluster_security_group_id" {
  description = "Security group ids attached to the cluster control plane"
  value       = module.eks.cluster_security_group_id
}

output "cluster_name" {
  description = "Kubernetes Cluster Name"
  value       = module.eks.cluster_name
}

output "postgres_endpoint" {
  description = "RDS instance endpoint"
  value       = aws_db_instance.postgres.endpoint
}

output "redis_endpoint" {
  description = "ElastiCache replication group configuration endpoint"
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}
```

### 16. Helm Charts for Application Deployment

#### Main Helm Chart Structure

```
helm/
├── go-ai-poi/
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── values-production.yaml
│   ├── values-staging.yaml
│   └── templates/
│       ├── _helpers.tpl
│       ├── namespace.yaml
│       ├── configmap.yaml
│       ├── secret.yaml
│       ├── services/
│       │   ├── auth-service.yaml
│       │   ├── poi-service.yaml
│       │   ├── chat-service.yaml
│       │   └── ... (other services)
│       ├── observability/
│       │   ├── prometheus.yaml
│       │   ├── grafana.yaml
│       │   ├── tempo.yaml
│       │   └── loki.yaml
│       ├── ingress.yaml
│       └── hpa.yaml
```

#### Chart.yaml

```yaml
# helm/go-ai-poi/Chart.yaml
apiVersion: v2
name: go-ai-poi
description: A Helm chart for Go AI POI microservices platform
type: application
version: 0.1.0
appVersion: "1.0.0"

dependencies:
  - name: postgresql
    version: "12.12.10"
    repository: "https://charts.bitnami.com/bitnami"
    condition: postgresql.enabled
  - name: redis
    version: "18.1.5"
    repository: "https://charts.bitnami.com/bitnami"
    condition: redis.enabled
  - name: prometheus
    version: "25.6.0"
    repository: "https://prometheus-community.github.io/helm-charts"
    condition: observability.prometheus.enabled
  - name: grafana
    version: "7.0.6"
    repository: "https://grafana.github.io/helm-charts"
    condition: observability.grafana.enabled
  - name: tempo
    version: "1.7.1"
    repository: "https://grafana.github.io/helm-charts"
    condition: observability.tempo.enabled
  - name: loki
    version: "5.38.0"
    repository: "https://grafana.github.io/helm-charts"
    condition: observability.loki.enabled
```

#### Values.yaml

```yaml
# helm/go-ai-poi/values.yaml
global:
  environment: production
  imageRegistry: "your-registry.com"
  imageTag: "latest"
  imagePullPolicy: IfNotPresent

# Microservices configuration
services:
  auth:
    enabled: true
    replicaCount: 3
    image:
      repository: go-ai-poi/auth-service
    service:
      type: ClusterIP
      httpPort: 8001
      grpcPort: 9001
    resources:
      requests:
        memory: "256Mi"
        cpu: "100m"
      limits:
        memory: "512Mi"
        cpu: "500m"
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 10
      targetCPUUtilizationPercentage: 70

  poi:
    enabled: true
    replicaCount: 3
    image:
      repository: go-ai-poi/poi-service
    service:
      type: ClusterIP
      httpPort: 8002
      grpcPort: 9002
    resources:
      requests:
        memory: "512Mi"
        cpu: "200m"
      limits:
        memory: "1Gi"
        cpu: "1000m"
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 15
      targetCPUUtilizationPercentage: 70

  chat:
    enabled: true
    replicaCount: 3
    image:
      repository: go-ai-poi/chat-service
    service:
      type: ClusterIP
      httpPort: 8003
      grpcPort: 9003
    resources:
      requests:
        memory: "512Mi"
        cpu: "300m"
      limits:
        memory: "1Gi"
        cpu: "1500m"
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 20
      targetCPUUtilizationPercentage: 70

# Infrastructure
postgresql:
  enabled: true
  auth:
    postgresPassword: "loci123"
    username: "loci"
    password: "loci123"
    database: "loci"
  primary:
    persistence:
      enabled: true
      size: 20Gi
    resources:
      requests:
        memory: "1Gi"
        cpu: "500m"
      limits:
        memory: "4Gi"
        cpu: "2000m"

redis:
  enabled: true
  auth:
    enabled: false
  master:
    persistence:
      enabled: true
      size: 8Gi
    resources:
      requests:
        memory: "256Mi"
        cpu: "100m"
      limits:
        memory: "512Mi"
        cpu: "200m"

# Observability Stack
observability:
  prometheus:
    enabled: true
    server:
      persistentVolume:
        size: 10Gi
      resources:
        requests:
          memory: "512Mi"
          cpu: "200m"
        limits:
          memory: "2Gi"
          cpu: "1000m"

  grafana:
    enabled: true
    persistence:
      enabled: true
      size: 5Gi
    adminPassword: "admin"
    resources:
      requests:
        memory: "256Mi"
        cpu: "100m"
      limits:
        memory: "512Mi"
        cpu: "200m"

  tempo:
    enabled: true
    tempo:
      storage:
        trace:
          backend: local
          local:
            path: /var/tempo/traces

  loki:
    enabled: true
    loki:
      storage:
        type: filesystem

# Ingress
ingress:
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: api.your-domain.com
      paths:
        - path: /auth
          pathType: Prefix
          service: auth-service
          port: 8001
        - path: /poi
          pathType: Prefix
          service: poi-service
          port: 8002
        - path: /chat
          pathType: Prefix
          service: chat-service
          port: 8003
  tls:
    - secretName: go-ai-poi-tls
      hosts:
        - api.your-domain.com

# Monitoring Ingress
monitoringIngress:
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/auth-type: basic
    nginx.ingress.kubernetes.io/auth-secret: monitoring-auth
  hosts:
    - host: monitoring.your-domain.com
      paths:
        - path: /grafana
          pathType: Prefix
          service: grafana
          port: 3000
        - path: /prometheus
          pathType: Prefix
          service: prometheus-server
          port: 80
```

#### Service Template Example

```yaml
# helm/go-ai-poi/templates/services/auth-service.yaml
{{- if .Values.services.auth.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "go-ai-poi.fullname" . }}-auth
  labels:
    {{- include "go-ai-poi.labels" . | nindent 4 }}
    app.kubernetes.io/component: auth-service
spec:
  {{- if not .Values.services.auth.autoscaling.enabled }}
  replicas: {{ .Values.services.auth.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "go-ai-poi.selectorLabels" . | nindent 6 }}
      app.kubernetes.io/component: auth-service
  template:
    metadata:
      labels:
        {{- include "go-ai-poi.selectorLabels" . | nindent 8 }}
        app.kubernetes.io/component: auth-service
    spec:
      containers:
        - name: auth-service
          image: "{{ .Values.global.imageRegistry }}/{{ .Values.services.auth.image.repository }}:{{ .Values.global.imageTag }}"
          imagePullPolicy: {{ .Values.global.imagePullPolicy }}
          env:
            - name: SERVICE_NAME
              value: "auth-service"
            - name: SERVICE_PORT
              value: "{{ .Values.services.auth.service.httpPort }}"
            - name: GRPC_PORT
              value: "{{ .Values.services.auth.service.grpcPort }}"
            - name: POSTGRES_HOST
              value: {{ include "go-ai-poi.postgresql.fullname" . }}
            - name: POSTGRES_PORT
              value: "5432"
            - name: REDIS_HOST
              value: {{ include "go-ai-poi.redis.fullname" . }}-master
            - name: REDIS_PORT
              value: "6379"
          envFrom:
            - secretRef:
                name: {{ include "go-ai-poi.fullname" . }}-postgres-secret
          ports:
            - name: http
              containerPort: {{ .Values.services.auth.service.httpPort }}
              protocol: TCP
            - name: grpc
              containerPort: {{ .Values.services.auth.service.grpcPort }}
              protocol: TCP
            - name: pprof
              containerPort: 6060
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 30
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /ready
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            {{- toYaml .Values.services.auth.resources | nindent 12 }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "go-ai-poi.fullname" . }}-auth
  labels:
    {{- include "go-ai-poi.labels" . | nindent 4 }}
    app.kubernetes.io/component: auth-service
spec:
  type: {{ .Values.services.auth.service.type }}
  ports:
    - port: {{ .Values.services.auth.service.httpPort }}
      targetPort: http
      protocol: TCP
      name: http
    - port: {{ .Values.services.auth.service.grpcPort }}
      targetPort: grpc
      protocol: TCP
      name: grpc
  selector:
    {{- include "go-ai-poi.selectorLabels" . | nindent 4 }}
    app.kubernetes.io/component: auth-service
{{- if .Values.services.auth.autoscaling.enabled }}
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "go-ai-poi.fullname" . }}-auth
  labels:
    {{- include "go-ai-poi.labels" . | nindent 4 }}
    app.kubernetes.io/component: auth-service
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "go-ai-poi.fullname" . }}-auth
  minReplicas: {{ .Values.services.auth.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.services.auth.autoscaling.maxReplicas }}
  metrics:
    {{- if .Values.services.auth.autoscaling.targetCPUUtilizationPercentage }}
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.services.auth.autoscaling.targetCPUUtilizationPercentage }}
    {{- end }}
{{- end }}
{{- end }}
```

### 17. Deployment Automation

#### Terraform Deployment Script

```bash
#!/bin/bash
# scripts/deploy-infrastructure.sh

set -e

ENVIRONMENT=${1:-production}
AWS_REGION=${2:-us-west-2}

echo "Deploying infrastructure for environment: $ENVIRONMENT"

# Initialize Terraform
cd terraform
terraform init

# Plan the deployment
terraform plan \
  -var="environment=$ENVIRONMENT" \
  -var="aws_region=$AWS_REGION" \
  -out=tfplan

# Apply the plan
terraform apply tfplan

# Get outputs
CLUSTER_NAME=$(terraform output -raw cluster_name)
POSTGRES_ENDPOINT=$(terraform output -raw postgres_endpoint)
REDIS_ENDPOINT=$(terraform output -raw redis_endpoint)

# Update kubeconfig
aws eks update-kubeconfig --region $AWS_REGION --name $CLUSTER_NAME

# Install necessary Kubernetes addons
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Install NGINX Ingress Controller
helm upgrade --install ingress-nginx ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx \
  --namespace ingress-nginx --create-namespace

# Install cert-manager
helm upgrade --install cert-manager cert-manager \
  --repo https://charts.jetstack.io \
  --namespace cert-manager --create-namespace \
  --set installCRDs=true

echo "Infrastructure deployment completed!"
echo "Cluster Name: $CLUSTER_NAME"
echo "PostgreSQL Endpoint: $POSTGRES_ENDPOINT"
echo "Redis Endpoint: $REDIS_ENDPOINT"
```

#### Helm Deployment Script

```bash
#!/bin/bash
# scripts/deploy-application.sh

set -e

ENVIRONMENT=${1:-production}
NAMESPACE=${2:-go-ai-poi}

echo "Deploying application for environment: $ENVIRONMENT"

# Add Helm repositories
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

# Deploy the application
helm upgrade --install go-ai-poi ./helm/go-ai-poi \
  --namespace $NAMESPACE \
  --create-namespace \
  --values ./helm/go-ai-poi/values-$ENVIRONMENT.yaml \
  --wait \
  --timeout 10m

echo "Application deployment completed!"

# Get service endpoints
kubectl get ingress -n $NAMESPACE
kubectl get services -n $NAMESPACE
```

#### CI/CD Pipeline Example (GitHub Actions)

```yaml
# .github/workflows/deploy.yml
name: Deploy to Kubernetes

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  AWS_REGION: us-west-2
  EKS_CLUSTER_NAME: go-ai-poi-cluster

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run tests
        run: make test

  build:
    needs: test
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service: [auth, poi, chat, lists, users, admin, city, interests, profiles, recents, reviews, statistics, tags]
    steps:
      - uses: actions/checkout@v4
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ env.AWS_REGION }}
      
      - name: Login to Amazon ECR
        uses: aws-actions/amazon-ecr-login@v2
      
      - name: Build and push Docker image
        env:
          ECR_REGISTRY: ${{ steps.login-ecr.outputs.registry }}
          ECR_REPOSITORY: go-ai-poi/${{ matrix.service }}-service
          IMAGE_TAG: ${{ github.sha }}
        run: |
          docker build -t $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG ./internal/domain/${{ matrix.service }}
          docker push $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG

  deploy-infrastructure:
    if: github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Setup Terraform
        uses: hashicorp/setup-terraform@v3
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ env.AWS_REGION }}
      
      - name: Terraform Init
        run: cd terraform && terraform init
      
      - name: Terraform Plan
        run: |
          cd terraform
          terraform plan -var="postgres_password=${{ secrets.POSTGRES_PASSWORD }}"
      
      - name: Terraform Apply
        run: |
          cd terraform
          terraform apply -auto-approve -var="postgres_password=${{ secrets.POSTGRES_PASSWORD }}"

  deploy-application:
    if: github.ref == 'refs/heads/main'
    needs: [deploy-infrastructure]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ env.AWS_REGION }}
      
      - name: Update kubeconfig
        run: aws eks update-kubeconfig --region ${{ env.AWS_REGION }} --name ${{ env.EKS_CLUSTER_NAME }}
      
      - name: Setup Helm
        uses: azure/setup-helm@v3
        with:
          version: '3.12.0'
      
      - name: Deploy with Helm
        run: |
          helm repo add bitnami https://charts.bitnami.com/bitnami
          helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
          helm repo add grafana https://grafana.github.io/helm-charts
          helm repo update
          
          helm upgrade --install go-ai-poi ./helm/go-ai-poi \
            --namespace go-ai-poi \
            --create-namespace \
            --values ./helm/go-ai-poi/values-production.yaml \
            --set global.imageTag=${{ github.sha }} \
            --wait \
            --timeout 15m
```

### 18. Operational Considerations

- **Blue-Green Deployments**: Use ArgoCD or Flux for GitOps
- **Database Migrations**: Run as Kubernetes Jobs before deployments
- **Backup Strategy**: Implement regular database and configuration backups
- **Disaster Recovery**: Plan for multi-region deployments
- **Cost Optimization**: Use spot instances and resource optimization tools
- **Infrastructure Monitoring**: Monitor Terraform state and AWS resources
- **Helm Chart Versioning**: Use semantic versioning for Helm charts
- **Secret Management**: Use AWS Secrets Manager or HashiCorp Vault

This comprehensive infrastructure setup provides a production-ready, scalable, and maintainable microservices platform using modern DevOps practices.