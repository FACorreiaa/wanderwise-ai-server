# GCP & Cloudflare Integration Guide

This document outlines the integration strategy for deploying the Go AI POI application across Google Cloud Platform (GCP) and Cloudflare.

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Cloudflare     │    │  Google Cloud   │    │  Google Cloud   │
│  Workers        │────│  Run/GKE        │────│  SQL/Firestore │
│  (Frontend)     │    │  (Go Server)    │    │  (Database)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 1. Database Layer - Google Cloud SQL

### 1.1 Cloud SQL PostgreSQL Setup

**Create Cloud SQL Instance:**
```bash
gcloud sql instances create go-ai-poi-db \
    --database-version=POSTGRES_15 \
    --tier=db-f1-micro \
    --region=us-central1 \
    --storage-type=SSD \
    --storage-size=10GB \
    --storage-auto-increase \
    --backup-start-time=03:00 \
    --enable-bin-log \
    --maintenance-window-day=SUN \
    --maintenance-window-hour=4
```

**Create Database and User:**
```bash
# Create database
gcloud sql databases create poi_db --instance=go-ai-poi-db

# Create user
gcloud sql users create poi_user \
    --instance=go-ai-poi-db \
    --password=your-secure-password
```

**Connection Configuration:**
```yaml
# config.yml
database:
  host: /cloudsql/your-project:us-central1:go-ai-poi-db
  port: 5432
  name: poi_db
  user: poi_user
  password: ${DB_PASSWORD}
  sslmode: require
  max_connections: 10
  max_idle_connections: 5
```

### 1.2 Database Migration Strategy

**Cloud Build Migration Pipeline:**
```yaml
# cloudbuild-migration.yaml
steps:
  - name: 'gcr.io/cloud-builders/go'
    env:
      - 'CGO_ENABLED=0'
      - 'GOOS=linux'
    args:
      - 'build'
      - '-o'
      - 'migrate'
      - './cmd/migrate'
  
  - name: 'gcr.io/cloud-sql-docker/gce-proxy:1.33.2'
    args:
      - '/cloud_sql_proxy'
      - '-instances=${_INSTANCE_CONNECTION_NAME}=tcp:5432'
    env:
      - 'GOOGLE_APPLICATION_CREDENTIALS=/workspace/service-account.json'
  
  - name: 'gcr.io/cloud-builders/go'
    args:
      - './migrate'
      - 'up'
    env:
      - 'DB_HOST=127.0.0.1'
      - 'DB_PORT=5432'
      - 'DB_NAME=${_DB_NAME}'
      - 'DB_USER=${_DB_USER}'
      - 'DB_PASSWORD=${_DB_PASSWORD}'

substitutions:
  _INSTANCE_CONNECTION_NAME: 'your-project:us-central1:go-ai-poi-db'
  _DB_NAME: 'poi_db'
  _DB_USER: 'poi_user'
```

## 2. Backend Server - Google Cloud Run

### 2.1 Containerization

**Dockerfile Optimization:**

```dockerfile
# Multi-stage build for production
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY ../go.mod go.sum ./
RUN go mod download

COPY .. .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/config.yml .

# Create non-root user
RUN adduser -D -s /bin/sh appuser
USER appuser

EXPOSE 8080
CMD ["./main"]
```

### 2.2 Cloud Run Deployment

**Cloud Run Service Configuration:**
```yaml
# cloudrun-service.yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: go-ai-poi-server
  annotations:
    run.googleapis.com/ingress: all
    run.googleapis.com/execution-environment: gen2
spec:
  template:
    metadata:
      annotations:
        run.googleapis.com/cloudsql-instances: your-project:us-central1:go-ai-poi-db
        run.googleapis.com/cpu-throttling: "false"
        autoscaling.knative.dev/maxScale: "10"
        autoscaling.knative.dev/minScale: "1"
    spec:
      containerConcurrency: 100
      timeoutSeconds: 300
      containers:
      - image: gcr.io/your-project/go-ai-poi-server:latest
        ports:
        - containerPort: 8080
        env:
        - name: PORT
          value: "8080"
        - name: DB_HOST
          value: "/cloudsql/your-project:us-central1:go-ai-poi-db"
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-password
              key: password
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: jwt-secret
              key: secret
        resources:
          limits:
            cpu: "1"
            memory: "512Mi"
          requests:
            cpu: "0.5"
            memory: "256Mi"
```

**Deployment Script:**
```bash
#!/bin/bash
# deploy-server.sh

PROJECT_ID="your-project-id"
REGION="us-central1"
SERVICE_NAME="go-ai-poi-server"

# Build and push container
docker build -t gcr.io/$PROJECT_ID/$SERVICE_NAME:latest .
docker push gcr.io/$PROJECT_ID/$SERVICE_NAME:latest

# Deploy to Cloud Run
gcloud run deploy $SERVICE_NAME \
    --image gcr.io/$PROJECT_ID/$SERVICE_NAME:latest \
    --platform managed \
    --region $REGION \
    --allow-unauthenticated \
    --set-cloudsql-instances $PROJECT_ID:$REGION:go-ai-poi-db \
    --set-env-vars "PORT=8080" \
    --set-secrets "DB_PASSWORD=db-password:latest,JWT_SECRET=jwt-secret:latest" \
    --memory 512Mi \
    --cpu 1 \
    --concurrency 100 \
    --max-instances 10 \
    --min-instances 1
```

### 2.3 Auto-scaling Configuration

**Custom Metrics & Scaling:**
```yaml
# hpa.yaml for GKE (alternative to Cloud Run)
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: go-ai-poi-server-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: go-ai-poi-server
  minReplicas: 2
  maxReplicas: 20
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

## 3. Frontend - Cloudflare Workers

### 3.1 SolidJS Build Configuration

**wrangler.toml:**
```toml
name = "go-ai-poi-frontend"
main = "dist/worker.js"
compatibility_date = "2024-01-15"
compatibility_flags = ["nodejs_compat"]

[build]
command = "npm run build:cf"
cwd = "."
watch_dir = "src"

[env.production]
vars = { API_BASE_URL = "https://go-ai-poi-server-xxx-uc.a.run.app" }

[env.staging]
vars = { API_BASE_URL = "https://staging-go-ai-poi-server-xxx-uc.a.run.app" }

[[env.production.routes]]
pattern = "poi.yourapp.com/*"
zone_name = "yourapp.com"

[[env.staging.routes]]
pattern = "staging-poi.yourapp.com/*"
zone_name = "yourapp.com"

# KV namespaces for caching
[[kv_namespaces]]
binding = "CACHE"
id = "your-kv-namespace-id"

# Analytics
[observability]
enabled = true
```

**Vite Configuration for Cloudflare Workers:**
```javascript
// vite.config.cf.js
import { defineConfig } from 'vite';
import solid from 'vite-plugin-solid';

export default defineConfig({
  plugins: [solid({ ssr: true })],
  build: {
    outDir: 'dist',
    ssr: true,
    rollupOptions: {
      input: 'src/entry-server.tsx',
      output: {
        format: 'es',
        entryFileNames: 'worker.js',
      },
    },
  },
  ssr: {
    target: 'webworker',
    noExternal: true,
  },
  define: {
    global: 'globalThis',
  },
});
```

**Worker Entry Point:**
```typescript
// src/entry-server.tsx
import { renderToString } from 'solid-js/web';
import App from './App';

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    
    // Handle API proxy
    if (url.pathname.startsWith('/api')) {
      return handleAPIProxy(request, env);
    }
    
    // Handle static assets
    if (url.pathname.startsWith('/assets')) {
      return handleStaticAssets(request, env);
    }
    
    // Render SolidJS app
    const html = renderToString(() => <App />);
    
    return new Response(html, {
      headers: {
        'Content-Type': 'text/html',
        'Cache-Control': 'public, max-age=3600',
      },
    });
  },
};

async function handleAPIProxy(request: Request, env: Env): Promise<Response> {
  const url = new URL(request.url);
  const apiUrl = `${env.API_BASE_URL}${url.pathname}${url.search}`;
  
  // Forward request to backend
  const response = await fetch(apiUrl, {
    method: request.method,
    headers: request.headers,
    body: request.body,
  });
  
  // Add CORS headers
  const corsHeaders = {
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  };
  
  return new Response(response.body, {
    status: response.status,
    headers: { ...response.headers, ...corsHeaders },
  });
}
```

### 3.2 Deployment Pipeline

**GitHub Actions for Cloudflare:**
```yaml
# .github/workflows/deploy-frontend.yml
name: Deploy Frontend to Cloudflare Workers

on:
  push:
    branches: [main]
    paths: ['go-ai-poi-client/**']

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'
          cache: 'npm'
          cache-dependency-path: go-ai-poi-client/package-lock.json
      
      - name: Install dependencies
        working-directory: go-ai-poi-client
        run: npm ci
      
      - name: Build for Cloudflare Workers
        working-directory: go-ai-poi-client
        run: npm run build:cf
        env:
          VITE_API_BASE_URL: ${{ secrets.API_BASE_URL }}
      
      - name: Deploy to Cloudflare Workers
        working-directory: go-ai-poi-client
        run: npx wrangler deploy
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
```

## 4. Networking & Security

### 4.1 Custom Domain & SSL

**Cloudflare Custom Domain:**
```bash
# Add custom domain to Cloudflare Workers
wrangler route add "poi.yourapp.com/*" go-ai-poi-frontend

# Configure DNS records
# A record: poi.yourapp.com -> Cloudflare Workers IP
# CNAME record: api.poi.yourapp.com -> go-ai-poi-server-xxx-uc.a.run.app
```

### 4.2 Security Configuration

**Cloud Armor (DDoS Protection):**
```yaml
# security-policy.yaml
apiVersion: compute/v1
kind: SecurityPolicy
metadata:
  name: go-ai-poi-security-policy
spec:
  rules:
  - priority: 1000
    match:
      config:
        srcIpRanges: ["*"]
    action: "allow"
    rateLimitOptions:
      rateLimitThreshold:
        count: 100
        intervalSec: 60
      banThreshold:
        count: 1000
        intervalSec: 600
      banDurationSec: 600
```

**IAM Roles & Service Accounts:**
```bash
# Create service account for Cloud Run
gcloud iam service-accounts create go-ai-poi-server \
    --display-name="Go AI POI Server"

# Grant necessary permissions
gcloud projects add-iam-policy-binding your-project-id \
    --member="serviceAccount:go-ai-poi-server@your-project-id.iam.gserviceaccount.com" \
    --role="roles/cloudsql.client"

gcloud projects add-iam-policy-binding your-project-id \
    --member="serviceAccount:go-ai-poi-server@your-project-id.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"
```

## 5. Monitoring & Observability

### 5.1 Cloud Monitoring Setup

**Monitoring Configuration:**
```yaml
# monitoring.yaml
alertPolicy:
  displayName: "Go AI POI High Error Rate"
  conditions:
  - displayName: "Error rate > 5%"
    conditionThreshold:
      filter: 'resource.type="cloud_run_revision" AND resource.labels.service_name="go-ai-poi-server"'
      comparison: COMPARISON_GREATER_THAN
      thresholdValue: 0.05
      duration: 300s
  notificationChannels:
  - projects/your-project-id/notificationChannels/your-channel-id
```

### 5.2 Logging Strategy

**Structured Logging:**
```go
// Enhanced logging in your Go server
import (
    "log/slog"
    "cloud.google.com/go/logging"
)

func setupCloudLogging() *slog.Logger {
    client, err := logging.NewClient(context.Background(), "your-project-id")
    if err != nil {
        log.Fatal(err)
    }
    
    logger := client.Logger("go-ai-poi-server")
    
    return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
}
```

## 6. CI/CD Pipeline

### 6.1 Complete Deployment Pipeline

**Cloud Build Configuration:**
```yaml
# cloudbuild.yaml
steps:
  # Test backend
  - name: 'golang:1.21'
    entrypoint: 'go'
    args: ['test', './...']
    dir: 'go-ai-poi-server'
  
  # Build backend container
  - name: 'gcr.io/cloud-builders/docker'
    args: ['build', '-t', 'gcr.io/$PROJECT_ID/go-ai-poi-server:$COMMIT_SHA', 'go-ai-poi-server']
  
  # Push container
  - name: 'gcr.io/cloud-builders/docker'
    args: ['push', 'gcr.io/$PROJECT_ID/go-ai-poi-server:$COMMIT_SHA']
  
  # Deploy to Cloud Run
  - name: 'gcr.io/google.com/cloudsdktool/cloud-sdk'
    entrypoint: 'gcloud'
    args:
    - 'run'
    - 'deploy'
    - 'go-ai-poi-server'
    - '--image'
    - 'gcr.io/$PROJECT_ID/go-ai-poi-server:$COMMIT_SHA'
    - '--region'
    - 'us-central1'
    - '--platform'
    - 'managed'
    - '--allow-unauthenticated'
  
  # Build frontend
  - name: 'node:18'
    entrypoint: 'npm'
    args: ['ci']
    dir: 'go-ai-poi-client'
  
  - name: 'node:18'
    entrypoint: 'npm'
    args: ['run', 'build:cf']
    dir: 'go-ai-poi-client'
    env:
    - 'VITE_API_BASE_URL=https://go-ai-poi-server-xxx-uc.a.run.app'
  
  # Deploy frontend to Cloudflare
  - name: 'node:18'
    entrypoint: 'npx'
    args: ['wrangler', 'deploy']
    dir: 'go-ai-poi-client'
    secretEnv: ['CLOUDFLARE_API_TOKEN']

availableSecrets:
  secretManager:
  - versionName: projects/$PROJECT_ID/secrets/cloudflare-api-token/versions/latest
    env: 'CLOUDFLARE_API_TOKEN'
```

## 7. Cost Optimization

### 7.1 Resource Optimization

**Cloud Run Cost Controls:**
```bash
# Set budget alerts
gcloud billing budgets create \
    --billing-account=your-billing-account \
    --display-name="Go AI POI Budget" \
    --budget-amount=100USD \
    --threshold-percent=80,90,100
```

**Database Cost Management:**
```sql
-- Optimize database queries
CREATE INDEX CONCURRENTLY idx_llm_interactions_user_city 
ON llm_interactions(user_id, city_name, created_at DESC);

CREATE INDEX CONCURRENTLY idx_poi_details_interaction 
ON poi_details(llm_interaction_id);

-- Partition large tables
CREATE TABLE llm_interactions_2024 PARTITION OF llm_interactions
FOR VALUES FROM ('2024-01-01') TO ('2025-01-01');
```

## 8. Disaster Recovery

### 8.1 Backup Strategy

**Automated Backups:**
```bash
# Cloud SQL automated backups
gcloud sql instances patch go-ai-poi-db \
    --backup-start-time=03:00 \
    --retained-backups-count=30 \
    --retained-transaction-log-days=7
```

**Cross-Region Replication:**
```bash
# Create read replica in different region
gcloud sql instances create go-ai-poi-db-replica \
    --master-instance-name=go-ai-poi-db \
    --tier=db-f1-micro \
    --region=us-east1 \
    --replica-type=READ
```

## 9. Performance Optimization & Cost Minimization

### 9.1 Load Balancing Strategy

**Google Cloud Load Balancer Configuration:**
```yaml
# load-balancer.yaml
apiVersion: networking.gke.io/v1
kind: ManagedCertificate
metadata:
  name: go-ai-poi-ssl-cert
spec:
  domains:
    - api.poi.yourapp.com
    - backend.poi.yourapp.com
---
apiVersion: v1
kind: Service
metadata:
  name: go-ai-poi-backend-service
  annotations:
    cloud.google.com/neg: '{"ingress": true}'
    cloud.google.com/backend-config: '{"default": "go-ai-poi-backend-config"}'
spec:
  type: NodePort
  selector:
    app: go-ai-poi-server
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: go-ai-poi-ingress
  annotations:
    kubernetes.io/ingress.global-static-ip-name: "go-ai-poi-ip"
    networking.gke.io/managed-certificates: "go-ai-poi-ssl-cert"
    kubernetes.io/ingress.class: "gce"
    kubernetes.io/ingress.allow-http: "false"
spec:
  rules:
  - host: api.poi.yourapp.com
    http:
      paths:
      - path: /*
        pathType: ImplementationSpecific
        backend:
          service:
            name: go-ai-poi-backend-service
            port:
              number: 8080
```

**Backend Configuration for Load Balancer:**
```yaml
# backend-config.yaml
apiVersion: cloud.google.com/v1
kind: BackendConfig
metadata:
  name: go-ai-poi-backend-config
spec:
  timeoutSec: 30
  connectionDraining:
    drainingTimeoutSec: 60
  healthCheck:
    checkIntervalSec: 10
    timeoutSec: 5
    healthyThreshold: 1
    unhealthyThreshold: 3
    type: HTTP
    requestPath: /health
    port: 8080
  cdn:
    enabled: true
    cachePolicy:
      includeHost: true
      includeProtocol: true
      includeQueryString: false
    negativeCaching: true
    negativeCachingPolicy:
    - code: 404
      ttl: 120
    - code: 500
      ttl: 60
```

### 9.2 NGINX Reverse Proxy (Alternative Cost-Effective Setup)

**NGINX Configuration for Compute Engine:**
```nginx
# /etc/nginx/sites-available/go-ai-poi
upstream go_backend {
    least_conn;
    server 127.0.0.1:8080 max_fails=3 fail_timeout=30s;
    server 127.0.0.1:8081 max_fails=3 fail_timeout=30s backup;
    keepalive 32;
}

# Rate limiting
limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/s;

# Cache zones
proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=api_cache:10m max_size=1g 
                 inactive=60m use_temp_path=off;

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name api.poi.yourapp.com;

    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/api.poi.yourapp.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.poi.yourapp.com/privkey.pem;
    ssl_session_timeout 1d;
    ssl_session_cache shared:MozTLS:10m;
    ssl_session_tickets off;
    
    # Modern configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    # HSTS
    add_header Strict-Transport-Security "max-age=63072000" always;
    
    # Security headers
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "no-referrer-when-downgrade" always;
    add_header Content-Security-Policy "default-src 'self' http: https: ws: wss: data: blob: 'unsafe-inline'; frame-ancestors 'none';" always;

    # Gzip compression
    gzip on;
    gzip_vary on;
    gzip_min_length 10240;
    gzip_proxied expired no-cache no-store private must-revalidate auth;
    gzip_types
        text/plain
        text/css
        text/xml
        text/javascript
        application/javascript
        application/xml+rss
        application/json;

    # API endpoints with caching
    location /api/v1/cities {
        limit_req zone=api burst=20 nodelay;
        proxy_cache api_cache;
        proxy_cache_valid 200 10m;
        proxy_cache_valid 404 1m;
        proxy_cache_key "$scheme$request_method$host$request_uri";
        add_header X-Cache-Status $upstream_cache_status;
        
        proxy_pass http://go_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        proxy_connect_timeout 5s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Authentication endpoints (no caching, stricter rate limiting)
    location /api/v1/auth {
        limit_req zone=auth burst=10 nodelay;
        
        proxy_pass http://go_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    # Real-time endpoints (WebSocket support)
    location /api/v1/ws {
        proxy_pass http://go_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400;
    }

    # Health check endpoint
    location /health {
        access_log off;
        proxy_pass http://go_backend;
        proxy_connect_timeout 2s;
        proxy_send_timeout 2s;
        proxy_read_timeout 2s;
    }

    # Static file serving (if needed)
    location /static/ {
        alias /var/www/static/;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}

# HTTP to HTTPS redirect
server {
    listen 80;
    listen [::]:80;
    server_name api.poi.yourapp.com;
    return 301 https://$server_name$request_uri;
}
```

### 9.3 Automated SSL/TLS with Certbot

**Certbot Setup Script:**
```bash
#!/bin/bash
# setup-ssl.sh

# Install Certbot
sudo apt update
sudo apt install -y certbot python3-certbot-nginx

# Create certificates
sudo certbot --nginx \
    -d api.poi.yourapp.com \
    -d backend.poi.yourapp.com \
    --non-interactive \
    --agree-tos \
    --email admin@yourapp.com \
    --redirect

# Setup auto-renewal
sudo crontab -l | { cat; echo "0 12 * * * /usr/bin/certbot renew --quiet"; } | sudo crontab -

# Test renewal
sudo certbot renew --dry-run

# Reload nginx after renewal
echo '#!/bin/bash
certbot renew --quiet --deploy-hook "systemctl reload nginx"
' | sudo tee /etc/cron.daily/certbot-renew
sudo chmod +x /etc/cron.daily/certbot-renew
```

**Let's Encrypt with DNS Challenge (for wildcard certs):**
```bash
#!/bin/bash
# setup-wildcard-ssl.sh

# Install Cloudflare plugin
sudo apt install -y python3-certbot-dns-cloudflare

# Create Cloudflare credentials
sudo mkdir -p /etc/letsencrypt
echo "dns_cloudflare_api_token = your-cloudflare-api-token" | sudo tee /etc/letsencrypt/cloudflare.ini
sudo chmod 600 /etc/letsencrypt/cloudflare.ini

# Get wildcard certificate
sudo certbot certonly \
    --dns-cloudflare \
    --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
    -d "*.poi.yourapp.com" \
    -d poi.yourapp.com \
    --non-interactive \
    --agree-tos \
    --email admin@yourapp.com
```

### 9.4 Cost-Optimized Compute Engine Setup

**VM Instance Configuration:**
```bash
#!/bin/bash
# create-optimized-vm.sh

# Create cost-optimized VM with preemptible instance
gcloud compute instances create go-ai-poi-server \
    --zone=us-central1-a \
    --machine-type=e2-small \
    --network-tier=STANDARD \
    --maintenance-policy=MIGRATE \
    --preemptible \
    --image-family=ubuntu-2004-lts \
    --image-project=ubuntu-os-cloud \
    --boot-disk-size=20GB \
    --boot-disk-type=pd-standard \
    --boot-disk-device-name=go-ai-poi-server \
    --metadata-from-file startup-script=startup.sh \
    --tags=http-server,https-server

# Create instance template for auto-scaling
gcloud compute instance-templates create go-ai-poi-template \
    --machine-type=e2-small \
    --network-tier=STANDARD \
    --image-family=ubuntu-2004-lts \
    --image-project=ubuntu-os-cloud \
    --boot-disk-size=20GB \
    --boot-disk-type=pd-standard \
    --preemptible \
    --metadata-from-file startup-script=startup.sh \
    --tags=http-server,https-server

# Create managed instance group
gcloud compute instance-groups managed create go-ai-poi-group \
    --template=go-ai-poi-template \
    --size=1 \
    --zone=us-central1-a

# Setup auto-scaling
gcloud compute instance-groups managed set-autoscaling go-ai-poi-group \
    --zone=us-central1-a \
    --max-num-replicas=3 \
    --min-num-replicas=1 \
    --target-cpu-utilization=0.7 \
    --cool-down-period=60s
```

**Startup Script for VM:**
```bash
#!/bin/bash
# startup.sh

# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh
usermod -aG docker $USER

# Install NGINX
apt install -y nginx

# Install Certbot
apt install -y certbot python3-certbot-nginx

# Install monitoring agent
curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh
bash add-google-cloud-ops-agent-repo.sh --also-install

# Download and run application
docker pull gcr.io/your-project/go-ai-poi-server:latest
docker run -d \
    --name go-ai-poi-app \
    --restart unless-stopped \
    -p 127.0.0.1:8080:8080 \
    -e DB_HOST="/cloudsql/your-project:us-central1:go-ai-poi-db" \
    gcr.io/your-project/go-ai-poi-server:latest

# Setup NGINX configuration
curl -o /etc/nginx/sites-available/go-ai-poi \
    https://raw.githubusercontent.com/your-repo/configs/nginx.conf
ln -s /etc/nginx/sites-available/go-ai-poi /etc/nginx/sites-enabled/
rm /etc/nginx/sites-enabled/default
systemctl reload nginx

# Setup SSL
certbot --nginx -d api.poi.yourapp.com --non-interactive --agree-tos --email admin@yourapp.com
```

### 9.5 Advanced Caching Strategy

**Redis Cache Implementation:**
```go
// cache/redis.go
package cache

import (
    "context"
    "encoding/json"
    "time"
    "github.com/redis/go-redis/v9"
)

type RedisCache struct {
    client *redis.Client
}

func NewRedisCache(addr, password string) *RedisCache {
    rdb := redis.NewClient(&redis.Options{
        Addr:     addr,
        Password: password,
        DB:       0,
    })
    
    return &RedisCache{client: rdb}
}

func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
    json, err := json.Marshal(value)
    if err != nil {
        return err
    }
    
    return r.client.Set(ctx, key, json, ttl).Err()
}

func (r *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
    val, err := r.client.Get(ctx, key).Result()
    if err != nil {
        return err
    }
    
    return json.Unmarshal([]byte(val), dest)
}

// Cache middleware for Gin
func (r *RedisCache) CacheMiddleware(ttl time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.Request.URL.Path + "?" + c.Request.URL.RawQuery
        
        var cached interface{}
        if err := r.Get(c.Request.Context(), key, &cached); err == nil {
            c.JSON(200, cached)
            return
        }
        
        c.Next()
        
        if c.Writer.Status() == 200 {
            // Cache successful responses
            go func() {
                ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
                defer cancel()
                r.Set(ctx, key, c.Keys["response"], ttl)
            }()
        }
    }
}
```

**Memory Cache Configuration:**
```yaml
# redis.conf
maxmemory 128mb
maxmemory-policy allkeys-lru
save ""
appendonly no
tcp-keepalive 60
timeout 300
```

### 9.6 Content Delivery Network (CDN) Setup

**Cloudflare CDN Configuration:**
```javascript
// cloudflare-worker-cdn.js
export default {
  async fetch(request, env) {
    const cache = caches.default;
    const cacheKey = new Request(request.url, request);
    
    // Check cache first
    let response = await cache.match(cacheKey);
    if (response) {
      return response;
    }
    
    // If not in cache, fetch from origin
    response = await fetch(request);
    
    // Cache static assets for longer
    if (request.url.includes('/assets/') || request.url.includes('/static/')) {
      const cacheResponse = response.clone();
      cacheResponse.headers.set('Cache-Control', 'public, max-age=31536000');
      await cache.put(cacheKey, cacheResponse);
    }
    
    // Cache API responses for shorter time
    if (request.url.includes('/api/') && response.status === 200) {
      const cacheResponse = response.clone();
      cacheResponse.headers.set('Cache-Control', 'public, max-age=300');
      await cache.put(cacheKey, cacheResponse);
    }
    
    return response;
  }
};
```

### 9.7 Database Connection Pooling & Optimization

**PgBouncer Configuration:**
```ini
# /etc/pgbouncer/pgbouncer.ini
[databases]
poi_db = host=/cloudsql/your-project:us-central1:go-ai-poi-db port=5432 dbname=poi_db

[pgbouncer]
listen_port = 6432
listen_addr = 127.0.0.1
auth_type = md5
auth_file = /etc/pgbouncer/userlist.txt
pool_mode = transaction
server_reset_query = DISCARD ALL
max_client_conn = 100
default_pool_size = 20
reserve_pool_size = 5
reserve_pool_timeout = 5
server_round_robin = 1
ignore_startup_parameters = extra_float_digits
server_idle_timeout = 600
server_connect_timeout = 15
server_login_retry = 15
client_login_timeout = 60
autodb_idle_timeout = 3600
```

**Go Database Configuration:**
```go
// db/config.go
func NewOptimizedDB(config *Config) (*sql.DB, error) {
    dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
        config.Host, config.Port, config.User, config.Password, config.DBName)
    
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, err
    }
    
    // Connection pool settings
    db.SetMaxOpenConns(20)           // Maximum connections
    db.SetMaxIdleConns(10)           // Maximum idle connections
    db.SetConnMaxLifetime(time.Hour) // Connection lifetime
    db.SetConnMaxIdleTime(30 * time.Minute) // Idle timeout
    
    return db, nil
}
```

### 9.8 Monitoring & Cost Alerts

**Budget Alerts Setup:**
```bash
#!/bin/bash
# setup-cost-monitoring.sh

# Create budget with multiple thresholds
gcloud billing budgets create \
    --billing-account=$BILLING_ACCOUNT_ID \
    --display-name="Go AI POI Production Budget" \
    --budget-amount=50 \
    --threshold-percent=50,75,90,100 \
    --notification-channels=$NOTIFICATION_CHANNEL \
    --filter-projects=$PROJECT_ID
```

**Resource Usage Monitoring:**
```yaml
# monitoring-alert.yaml
displayName: "High Resource Usage Alert"
combiner: OR
conditions:
- displayName: "CPU usage > 80%"
  conditionThreshold:
    filter: 'resource.type="compute_instance"'
    comparison: COMPARISON_GREATER_THAN
    thresholdValue: 0.8
    duration: 300s
- displayName: "Memory usage > 85%"
  conditionThreshold:
    filter: 'resource.type="compute_instance"'
    comparison: COMPARISON_GREATER_THAN
    thresholdValue: 0.85
    duration: 300s
```

### 9.9 Auto-Scaling & Resource Optimization

**Horizontal Pod Autoscaler (HPA) Configuration:**
```yaml
# hpa-optimized.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: go-ai-poi-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: go-ai-poi-server
  minReplicas: 1
  maxReplicas: 5
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
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 30
```

### 9.10 Complete Cost Optimization Checklist

**Infrastructure Cost Optimization:**
- [ ] Use preemptible/spot instances for non-critical workloads
- [ ] Implement auto-scaling with proper min/max limits
- [ ] Use committed use discounts for predictable workloads
- [ ] Optimize VM machine types based on actual usage
- [ ] Use regional persistent disks instead of zonal when possible
- [ ] Implement proper resource requests and limits
- [ ] Use Cloud SQL read replicas only when necessary
- [ ] Enable Cloud SQL automatic storage increase
- [ ] Use standard storage tier for backups
- [ ] Implement proper logging retention policies

**Application Performance Optimization:**
- [ ] Implement connection pooling (PgBouncer)
- [ ] Use Redis for caching frequently accessed data
- [ ] Optimize database queries with proper indexing
- [ ] Implement response compression (gzip)
- [ ] Use CDN for static assets
- [ ] Implement API response caching
- [ ] Optimize Docker image sizes
- [ ] Use multi-stage builds
- [ ] Implement graceful shutdowns
- [ ] Monitor and optimize garbage collection

**Security & SSL Optimization:**
- [ ] Use Let's Encrypt for free SSL certificates
- [ ] Implement proper NGINX security headers
- [ ] Use Cloud Armor for DDoS protection
- [ ] Implement rate limiting at multiple levels
- [ ] Use service accounts with minimal permissions
- [ ] Implement proper secret management
- [ ] Regular security updates and patches
- [ ] Network segmentation and firewalls

---

This comprehensive optimization guide ensures maximum performance while maintaining minimal costs through strategic use of load balancing, caching, SSL automation, and resource optimization across your Google Cloud Platform and Cloudflare infrastructure.