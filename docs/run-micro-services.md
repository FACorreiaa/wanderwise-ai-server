The best and most standard tool for this job in a local development environment is **Docker Compose**.

Using Docker Compose solves several problems at once:
*   It starts all your services with a single command (`docker-compose up`).
*   It manages the networking between them, so they can easily communicate.
*   It aggregates the logs from all services into a single, interleaved stream.
*   It ensures a consistent environment for each service.

Here’s a step-by-step guide on how to set this up.

---

### Strategy: Using Docker and Docker Compose

The plan is to create a `Dockerfile` for each microservice and a single `docker-compose.yml` file at the root of your project to define and link them all together.

#### Step 1: Create a `Dockerfile` for Each Microservice

Each of your services (`users-auth`, `users-payment`, `chat`, `pois`) needs its own `Dockerfile`. Since they are all Go applications, you can use a nearly identical template for each one. This file tells Docker how to build a runnable image of your service.

**Example `Dockerfile` for the `users-auth` service:**
Place this file inside the `/users-auth` directory.

```dockerfile
# /users-auth/Dockerfile

# --- Build Stage ---
# Use an official Go image for building the application
FROM golang:1.22-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files to leverage Docker's layer caching.
# This step will only be re-run if these files change.
COPY go.mod go.sum ./

# Download dependencies into the vendor directory.
# This ensures that all dependencies are included in the build.
RUN go mod vendor

# Copy the rest of the application source code
COPY . .

# Build the Go application.
# -o /app/server creates the binary named 'server' in the /app directory.
# -ldflags="-w -s" makes the binary smaller by removing debug information.
# CGO_ENABLED=0 is important for creating a static binary that works in a minimal container.
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /app/server ./cmd/main.go


# --- Final Stage ---
# Use a minimal base image for the final container to keep it small and secure.
FROM alpine:latest

# Set the working directory
WORKDIR /app

# Copy the built binary from the 'builder' stage
COPY --from=builder /app/server .

# (Optional) If your service needs config files, copy them here
# COPY --from=builder /app/config.yml .

# Expose the port that your service will listen on
EXPOSE 8081

# The command to run when the container starts
CMD ["./server"]
```

**Adapt this `Dockerfile` for your other services.** The main things you might change are:
*   The `EXPOSE` port number (e.g., `8082` for `users-payment`, `8083` for `chat`, etc.).
*   The path to `main.go` if it's different (e.g., `./cmd/server/main.go`).

#### Step 2: Create a `docker-compose.yml` at the Project Root

This is the main orchestration file. Create a file named `docker-compose.yml` at the very root of your project.

```yaml
# /my-project/docker-compose.yml

version: '3.8'

services:
  # Define the users-auth service
  users-auth:
    build:
      context: ./services/users-auth  # Path to the directory with the Dockerfile
    ports:
      - "8081:8081" # Map host port 8081 to container port 8081
    environment:
      # Pass environment variables to your service
      - DATABASE_URL=postgres://user:password@postgres-db:5432/users_db
      - JWT_SECRET=your-secret-key
    restart: unless-stopped
    depends_on:
      - postgres-db # Optional: Make it wait for the database to be ready

  # Define the users-payment service
  users-payment:
    build:
      context: ./services/users-payment
    ports:
      - "8082:8082"
    environment:
      - DATABASE_URL=postgres://user:password@postgres-db:5432/payments_db
      - STRIPE_API_KEY=your-stripe-key
    restart: unless-stopped
    depends_on:
      - postgres-db

  # Define the chat service
  chat:
    build:
      context: ./services/chat
    ports:
      - "8083:8083"
    restart: unless-stopped

  # Define the pois service
  pois:
    build:
      context: ./services/pois
    ports:
      - "8084:8084"
    environment:
      - DATABASE_URL=postgres://user:password@postgres-db:5432/pois_db
    restart: unless-stopped
    depends_on:
      - postgres-db

  # Define a PostgreSQL database service (very useful for local dev)
  postgres-db:
    image: postgis/postgis:16-3.4 # Use a PostGIS image if you need it
    environment:
      - POSTGRES_USER=user
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=main_db # You can have multiple DBs
    ports:
      - "5432:5432" # Map host port 5432 to container port 5432
    volumes:
      - postgres_data:/var/lib/postgresql/data # Persist database data

# Define a named volume to persist data across container restarts
volumes:
  postgres_data:
```

**Key parts of `docker-compose.yml` explained:**

*   **`services:`**: Each key under this is a service you want to run.
*   **`build: context:`**: Tells Docker Compose where to find the `Dockerfile` for that service.
*   **`ports:`**: Maps a port on your local machine (`host:container`) to a port inside the container. This is how you access your services from your browser or tools like Postman.
*   **`environment:`**: A clean way to pass configuration (like database connection strings, API keys) to your services.
*   **Service Naming:** Inside the Docker network, your services can talk to each other using their service names. For example, the `users-auth` service can connect to the database using the hostname `postgres-db`.

#### Step 3: Launch Everything

Now for the easy part. Open a terminal at the root of your project (where the `docker-compose.yml` file is) and run:

```bash
docker-compose up --build
```

*   **`docker-compose up`**: Reads the `docker-compose.yml` file, builds the images for any services that have changed, and starts all the containers.
*   **`--build`**: Forces Docker to rebuild the images from your `Dockerfile`s, ensuring it picks up your latest code changes.

You will see the logs from all your services, plus the database, interleaved in your terminal, color-coded by service.

To stop all the services, simply press `Ctrl + C` in that terminal. To run them in the background, use `docker-compose up -d`.

This setup provides a powerful, reproducible, and industry-standard way to manage a multi-service application locally.