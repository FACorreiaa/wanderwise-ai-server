# TODO: Points of Interest Sharing & Payment Architecture

## 🗺️ Share Points of Interest on Google Maps

### Current Status
- [ ] Not implemented
- [ ] Need to define sharing mechanism
- [ ] Google Maps integration exists but no sharing functionality

### Implementation Options

#### Option 1: Direct Google Maps URLs
- [ ] Generate Google Maps URLs with coordinates
- [ ] Format: `https://maps.google.com/?q=<lat>,<lng>&z=<zoom>`
- [ ] Pros: Simple, no API required, works universally
- [ ] Cons: Limited customization, no branding

#### Option 2: Google Maps Embed API
- [ ] Use Google Maps Embed API for rich sharing
- [ ] Custom markers and info windows
- [ ] Pros: Better UX, custom styling, branded experience
- [ ] Cons: Requires API key, more complex

#### Option 3: Deep Links + Fallback
- [ ] Try Google Maps app deep links first
- [ ] Fallback to web URLs if app not installed
- [ ] Format: `google.navigation:q=<lat>,<lng>` (Android) or `maps://?q=<lat>,<lng>` (iOS)

### Required Tasks
- [ ] **Research**: Investigate Google Maps sharing best practices
- [ ] **Design**: Create share button UI components
- [ ] **Implementation**: Add share functionality to POI cards/details
- [ ] **Testing**: Test across different devices and platforms
- [ ] **Analytics**: Track sharing usage and success rates

### Technical Considerations
- [ ] Handle different device types (mobile vs desktop)
- [ ] Consider privacy implications of sharing location data
- [ ] Add share analytics to track popular POIs
- [ ] Implement sharing via native share API when available
- [ ] Add copy-to-clipboard fallback

### User Experience
- [ ] Share button on POI detail pages
- [ ] Share button on POI cards in lists
- [ ] Bulk sharing for saved lists/collections
- [ ] Custom share messages with POI descriptions
- [ ] Share via social media, messaging apps, email

---

## 💳 Payment Logic Architecture Decision

### Current Status
- [ ] Payment forms implemented in frontend (SolidJS)
- [ ] No backend payment processing implemented
- [ ] Need to decide on architecture approach

### Option 1: Frontend-Heavy (Client-Side) ⚠️
```
SolidJS → Payment Provider (Stripe/PayPal) → Webhook → Go Backend
```

#### Pros:
- [ ] Faster user experience
- [ ] Reduced server load
- [ ] Modern payment UX patterns

#### Cons:
- [ ] **Security Risk**: Sensitive logic on client
- [ ] **Compliance Issues**: Harder to maintain PCI compliance
- [ ] **Validation Risk**: Client-side validation can be bypassed
- [ ] **State Management**: Complex sync between client/server

### Option 2: Backend-Heavy (Server-Side) ✅ **RECOMMENDED**
```
SolidJS → Go Backend → Payment Provider → Go Backend → SolidJS
```

#### Pros:
- [ ] **Security**: Sensitive operations server-side
- [ ] **Compliance**: Easier PCI DSS compliance
- [ ] **Validation**: Server-side validation cannot be bypassed
- [ ] **Audit Trail**: Complete payment logging
- [ ] **Business Logic**: Centralized subscription management

#### Cons:
- [ ] Slightly higher latency
- [ ] More server resources required
- [ ] Additional API endpoints needed

### Option 3: Hybrid Approach ✅ **ALTERNATIVE**
```
SolidJS (UI/UX) → Go Backend (Validation/Processing) → Payment Provider
```

#### Implementation:
- [ ] Client handles UI/UX and form validation
- [ ] Client sends payment intent to backend
- [ ] Backend validates, processes, and communicates with payment provider
- [ ] Backend returns results to client for UI updates

### **RECOMMENDED ARCHITECTURE: Backend-Heavy**

#### Implementation Plan:

##### Phase 1: Backend Payment Infrastructure
- [ ] **Payment Service Layer** (`internal/api/payment/`)
    - [ ] `payment_service.go` - Core payment logic
    - [ ] `payment_handler.go` - HTTP handlers
    - [ ] `payment_repository.go` - Database operations
    - [ ] `subscription_service.go` - Subscription management

- [ ] **Database Schema**
  ```sql
  -- Subscriptions table
  CREATE TABLE subscriptions (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    plan_name VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL, -- active, cancelled, expired
    billing_period VARCHAR(10) NOT NULL, -- monthly, yearly
    amount_cents INTEGER NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    payment_provider VARCHAR(20) NOT NULL, -- stripe, paypal
    provider_subscription_id VARCHAR(255),
    current_period_start TIMESTAMP,
    current_period_end TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
  );

  -- Payment transactions table
  CREATE TABLE payment_transactions (
    id UUID PRIMARY KEY,
    subscription_id UUID REFERENCES subscriptions(id),
    amount_cents INTEGER NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    status VARCHAR(20) NOT NULL, -- pending, succeeded, failed
    payment_provider VARCHAR(20) NOT NULL,
    provider_transaction_id VARCHAR(255),
    payment_method_type VARCHAR(50), -- card, paypal, apple_pay
    created_at TIMESTAMP DEFAULT NOW()
  );
  ```

- [ ] **API Endpoints**
  ```go
  // Payment routes
  POST   /api/v1/subscriptions              // Create subscription
  GET    /api/v1/subscriptions              // Get user subscriptions
  PUT    /api/v1/subscriptions/{id}         // Update subscription
  DELETE /api/v1/subscriptions/{id}         // Cancel subscription
  POST   /api/v1/payments/webhooks          // Payment provider webhooks
  GET    /api/v1/payments/transactions      // Payment history
  ```

##### Phase 2: Payment Provider Integration
- [ ] **Stripe Integration**
    - [ ] Customer creation
    - [ ] Subscription management
    - [ ] Webhook handling
    - [ ] Payment method management

- [ ] **PayPal Integration** (if needed)
    - [ ] PayPal API integration
    - [ ] Subscription management
    - [ ] Webhook handling

##### Phase 3: Frontend Integration
- [ ] **SolidJS Payment Components**
    - [ ] Update payment forms to call backend APIs
    - [ ] Remove direct payment provider calls
    - [ ] Add proper error handling and loading states
    - [ ] Implement subscription management UI

- [ ] **Security Measures**
    - [ ] CSRF protection on payment endpoints
    - [ ] Rate limiting on payment attempts
    - [ ] Input validation and sanitization
    - [ ] Secure session management

##### Phase 4: Testing & Compliance
- [ ] **Testing**
    - [ ] Unit tests for payment services
    - [ ] Integration tests for payment flows
    - [ ] End-to-end testing of subscription lifecycle
    - [ ] Load testing for payment endpoints

- [ ] **Security & Compliance**
    - [ ] Security audit of payment flow
    - [ ] PCI DSS compliance review
    - [ ] GDPR compliance for payment data
    - [ ] Penetration testing

### Migration Strategy
1. [ ] **Phase 1**: Implement backend payment infrastructure
2. [ ] **Phase 2**: Create parallel payment flow (keep existing frontend)
3. [ ] **Phase 3**: Migrate frontend to use backend APIs
4. [ ] **Phase 4**: Remove client-side payment logic
5. [ ] **Phase 5**: Security audit and compliance verification

### Configuration
```go
// config/payment.go
type PaymentConfig struct {
    Provider     string // "stripe" or "paypal"
    StripeKey    string
    PayPalKey    string
    WebhookSecret string
    Currency     string // "USD"
    Environment  string // "development" or "production"
}
```

---

## 🔄 Next Steps Priority

### High Priority
1. [ ] **Decision**: Finalize payment architecture (Backend-heavy recommended)
2. [ ] **Implementation**: Start with backend payment infrastructure
3. [ ] **Research**: Google Maps sharing implementation options

### Medium Priority
1. [ ] **Design**: Create POI sharing UI components
2. [ ] **Implementation**: Payment provider integration (Stripe first)
3. [ ] **Testing**: Payment flow testing infrastructure

### Low Priority
1. [ ] **Enhancement**: Google Maps sharing with custom branding
2. [ ] **Analytics**: Sharing and payment analytics
3. [ ] **Optimization**: Payment performance optimization

---

## 📋 Definition of Done

### POI Sharing Feature
- [ ] Users can share individual POIs via Google Maps
- [ ] Users can share collections/lists of POIs
- [ ] Sharing works across different platforms (iOS, Android, Desktop)
- [ ] Analytics track sharing usage
- [ ] Feature is responsive and accessible

### Payment Architecture
- [ ] All payment processing happens server-side
- [ ] PCI DSS compliance maintained
- [ ] Comprehensive error handling and logging
- [ ] Subscription lifecycle fully managed
- [ ] Security audit passed
- [ ] Performance requirements met (<2s payment processing)

---

*Last Updated: 2025-01-11*
*Next Review: Weekly during implementation*