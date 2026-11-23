# Resto API Specification (v1)

This specification defines the production REST and WebSocket contracts for the multi-tenant Resto platform.

## 1. Global Headers & Multi-Tenancy

Every incoming API request is resolved against a restaurant tenant:

```http
X-Tenant-Slug: royal-spice
# or resolved from Host header: royal-spice.resto.com
Authorization: Bearer <jwt_token> (Optional for Guest Dine-In QR)
```

---

## 2. Authentication & User Endpoints

### 2.1 Staff / Owner Login
- **Endpoint:** `POST /api/v1/auth/login`
- **Request Body:**
  ```json
  {
    "username": "waiter_rajesh",
    "password": "secure_password"
  }
  ```
- **Response (200 OK):**
  ```json
  {
    "token": "eyJhbGciOiJIUzI1Ni...",
    "user": {
      "id": "e0000000-0000-0000-0000-000000000001",
      "tenant_id": "a0000000-0000-0000-0000-000000000001",
      "role": "waiter",
      "first_name": "Rajesh",
      "last_name": "Kumar"
    }
  }
  ```

### 2.2 Customer Guest Session (Zero-App Scan)
- **Endpoint:** `POST /api/v1/auth/guest-session`
- **Request Body:**
  ```json
  {
    "phone": "+919876543210",
    "name": "Arun",
    "table_token": "qr_token_table_1_royal_spice"
  }
  ```
- **Response (200 OK):**
  ```json
  {
    "guest_token": "eyJhbGciOi...",
    "table": {
      "id": "...",
      "table_number": "T-01"
    }
  }
  ```

---

## 3. Tenant Branding & Menu Endpoints

### 3.1 Get Tenant Configuration & Branding
- **Endpoint:** `GET /api/v1/tenant/config`
- **Response (200 OK):**
  ```json
  {
    "id": "a0000000-0000-0000-0000-000000000001",
    "name": "The Royal Spice Restaurant",
    "branding": {
      "primary_color": "#e11d48",
      "secondary_color": "#1e293b",
      "accent_color": "#fbbf24",
      "font_family": "Inter",
      "logo_url": "https://..."
    },
    "settings": {
      "currency_symbol": "₹",
      "tax_gst_percentage": 5.0,
      "enable_dine_in": true,
      "enable_delivery": true
    }
  }
  ```

### 3.2 Get Menu by Category
- **Endpoint:** `GET /api/v1/menu`
- **Response (200 OK):**
  ```json
  [
    {
      "category_id": "c0000000-0000-0000-0000-000000000001",
      "category_name": "Starters",
      "items": [
        {
          "id": "item-uuid-1",
          "name": "Paneer Tikka",
          "price": 280.00,
          "food_type": "VEG",
          "is_available": true,
          "preparation_time_mins": 15
        }
      ]
    }
  ]
  ```

---

## 4. Order Management & Granular Pipeline

### 4.1 Place Order (Dine-In or Delivery)
- **Endpoint:** `POST /api/v1/orders`
- **Request Body:**
  ```json
  {
    "order_type": "DINE_IN", // or "DELIVERY"
    "table_id": "table-uuid", // Required for DINE_IN
    "delivery_address": "Flat 302, Green Glen Layout, Bangalore", // Required for DELIVERY
    "customer_phone": "+919876543210",
    "customer_name": "Arun",
    "items": [
      {
        "item_id": "item-uuid-1",
        "quantity": 2,
        "special_instructions": "Make it spicy, no onion"
      }
    ]
  }
  ```
- **Response (201 Created):**
  ```json
  {
    "order_id": "ord_1029384",
    "order_number": "RSP-104",
    "status": "PLACED",
    "subtotal": 560.00,
    "tax_amount": 28.00,
    "total_amount": 588.00,
    "tracking_url": "https://royal-spice.resto.com/track/ord_1029384"
  }
  ```

### 4.2 Update Order Status (Waiter / Kitchen / Driver)
- **Endpoint:** `PATCH /api/v1/orders/:order_id/status`
- **Request Body:**
  ```json
  {
    "status": "IN_COOKING",
    "note": "Chef started tandoor preparation"
  }
  ```
- **Allowed Status Transitions:**
  - `PLACED` → `WAITER_CONFIRMED`
  - `WAITER_CONFIRMED` → `CHEF_ACCEPTED`
  - `CHEF_ACCEPTED` → `IN_COOKING`
  - `IN_COOKING` → `BEING_PREPARED`
  - `BEING_PREPARED` → `PLATING`
  - `PLATING` → `READY_TO_SERVE`
  - `READY_TO_SERVE` → `SERVED` (Dine-In) or `DRIVER_ASSIGNED` (Delivery)
  - `DRIVER_ASSIGNED` → `OUT_FOR_DELIVERY`
  - `OUT_FOR_DELIVERY` → `DELIVERED`

---

## 5. Live Tracking & Delivery Map Streaming

### 5.1 Driver Location Ingestion (WebSocket)
- **Protocol:** `WSS`
- **Endpoint:** `wss://api.resto.com/ws/driver/location`
- **Auth:** Bearer Token (Driver role)
- **Message Payload (Sent by Driver PWA every 3-5 seconds):**
  ```json
  {
    "driver_id": "driver-uuid",
    "order_id": "ord_1029384",
    "latitude": 12.9716,
    "longitude": 77.5946,
    "heading": 87.5,
    "speed": 24.2
  }
  ```

### 5.2 Customer Live Map Stream (WebSocket / SSE)
- **Protocol:** `WSS` or `SSE`
- **Endpoint:** `GET /api/v1/orders/:order_id/track/stream` (SSE) or `wss://api.resto.com/ws/orders/:order_id/track`
- **Event Payload:**
  ```json
  {
    "order_id": "ord_1029384",
    "status": "OUT_FOR_DELIVERY",
    "driver": {
      "name": "Suresh Kumar",
      "phone": "+919812345678",
      "current_lat": 12.9725,
      "current_lng": 77.5980,
      "heading": 92.0
    },
    "destination": {
      "lat": 12.9750,
      "lng": 77.6050
    },
    "estimated_minutes": 8
  }
  ```

---

## 6. WhatsApp Cloud API Webhook & Notification Engine

- **Trigger:** Any status update to `CHEF_ACCEPTED`, `IN_COOKING`, `PLATING`, `READY_TO_SERVE`, or `OUT_FOR_DELIVERY`.
- **Payload Sent to Meta Cloud API:**
  ```json
  {
    "messaging_product": "whatsapp",
    "to": "+919876543210",
    "type": "template",
    "template": {
      "name": "order_status_update",
      "language": { "code": "en" },
      "components": [
        {
          "type": "body",
          "parameters": [
            { "type": "text", "text": "Arun" },
            { "type": "text", "text": "RSP-104" },
            { "type": "text", "text": "Your food is being plated! 🍽️" }
          ]
        },
        {
          "type": "button",
          "sub_type": "url",
          "index": 0,
          "parameters": [
            { "type": "text", "text": "ord_1029384" }
          ]
        }
      ]
    }
  }
  ```
