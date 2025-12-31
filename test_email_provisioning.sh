#!/bin/bash

# Email-Based Provisioning Test Script
# Tests the complete flow: super_admin creates tenant + pre-provisions tenant_admin

set -e

API_URL="http://localhost:8080"
ADMIN_EMAIL="test-admin-1766928588@homgym.app"  # Fixed email for testing

echo "🧪 Email-Based Provisioning Flow Test"
echo "======================================="
echo ""

# Step 1: Get Firebase token for super_admin
echo "📝 Step 1: Getting Firebase token for super_admin..."
FIREBASE_OUTPUT=$(python3 get_firebase_token.py 2>&1)
FIREBASE_TOKEN=$(echo "$FIREBASE_OUTPUT" | grep -A 1 "Your ID Token" | tail -1 | tr -d ' ')

if [ -z "$FIREBASE_TOKEN" ] || [[ ! "$FIREBASE_TOKEN" =~ ^eyJ ]]; then
    echo "❌ Failed to get valid Firebase token"
    exit 1
fi

echo "✅ Firebase token obtained"
echo ""

# Step 2: Login as super_admin
echo "📝 Step 2: Login as super_admin..."
SUPER_ADMIN_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Authorization: Bearer $FIREBASE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requested_role": "super_admin"}')

SUPER_ADMIN_TOKEN=$(echo $SUPER_ADMIN_RESPONSE | jq -r '.token')

if [ -z "$SUPER_ADMIN_TOKEN" ] || [ "$SUPER_ADMIN_TOKEN" = "null" ]; then
    echo "❌ Failed to get super_admin token"
    echo "Response: $SUPER_ADMIN_RESPONSE"
    exit 1
fi

echo "✅ Logged in as super_admin"
echo "   Token: ${SUPER_ADMIN_TOKEN:0:50}..."
echo ""

# Step 3: Create HOM Fitness tenant
echo "📝 Step 3: Creating HOM Fitness tenant..."
TENANT_RESPONSE=$(curl -s -X POST "$API_URL/v1/platform/tenants" \
  -H "Authorization: Bearer $SUPER_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "HOM Fitness",
    "join_code": "HOM-TEST-'$(date +%s)'",
    "logo_url": "https://homfitness.com/logo.png",
    "ai_settings": {
      "tone": "Encouraging",
      "style": "Detailed",
      "persona": "Supportive Head Coach"
    }
  }')

TENANT_ID=$(echo $TENANT_RESPONSE | jq -r '.id')
TENANT_NAME=$(echo $TENANT_RESPONSE | jq -r '.name')

if [ -z "$TENANT_ID" ] || [ "$TENANT_ID" = "null" ]; then
    echo "❌ Failed to create tenant"
    echo "Response: $TENANT_RESPONSE"
    exit 1
fi

echo "✅ Tenant created successfully"
echo "   Tenant ID: $TENANT_ID"
echo "   Tenant Name: $TENANT_NAME"
echo ""

# Step 4a: Delete existing user with this email (if any)
echo "📝 Step 4a: Checking for existing user with email $ADMIN_EMAIL..."
ALL_USERS=$(curl -s -X GET "$API_URL/v1/platform/users" \
  -H "Authorization: Bearer $SUPER_ADMIN_TOKEN")

EXISTING_USER_ID=$(echo $ALL_USERS | jq -r '.[] | select(.email == "'$ADMIN_EMAIL'") | .id')

if [ -n "$EXISTING_USER_ID" ] && [ "$EXISTING_USER_ID" != "null" ]; then
    echo "   Found existing user (ID: $EXISTING_USER_ID), deleting..."
    curl -s -X DELETE "$API_URL/v1/platform/users/$EXISTING_USER_ID" \
      -H "Authorization: Bearer $SUPER_ADMIN_TOKEN"
    echo "   ✅ Deleted old user"
else
    echo "   No existing user found"
fi
echo ""

# Step 4b: Pre-provision tenant_admin (EMAIL ONLY - no firebase_uid!)
echo "📝 Step 4b: Pre-provisioning tenant_admin with EMAIL ONLY..."

# Try to create the user
ADMIN_CREATE_RESPONSE=$(curl -s -X POST "$API_URL/v1/platform/users" \
  -H "Authorization: Bearer $SUPER_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "'$ADMIN_EMAIL'",
    "name": "Test Admin",
    "roles": ["tenant_admin"],
    "tenant_id": "'$TENANT_ID'"
  }')

ADMIN_USER_ID=$(echo $ADMIN_CREATE_RESPONSE | jq -r '.id')
ADMIN_FIREBASE_UID=$(echo $ADMIN_CREATE_RESPONSE | jq -r '.firebase_uid')

if [ "$ADMIN_USER_ID" = "null" ] || [ -z "$ADMIN_USER_ID" ]; then
    echo "❌ User creation failed!"
    echo "API Response: $ADMIN_CREATE_RESPONSE"
    exit 1
fi

echo "✅ Tenant admin created"
echo "   User ID: $ADMIN_USER_ID"
echo "   Email: $ADMIN_EMAIL"
echo "   Tenant ID: $TENANT_ID"
echo "   Firebase UID: $ADMIN_FIREBASE_UID"
echo ""

# Step 5: Verify firebase_uid is empty
if [ "$ADMIN_FIREBASE_UID" = "" ] || [ "$ADMIN_FIREBASE_UID" = "null" ]; then
    echo "✅ PASS: firebase_uid is empty (ready for linking)"
else
    echo "❌ FAIL: firebase_uid should be empty but got: $ADMIN_FIREBASE_UID"
    exit 1
fi

echo ""
echo "======================================="
echo "🎯 Test Summary"
echo "======================================="
echo "✅ Super admin login"
echo "✅ Tenant creation via /v1/platform/tenants"
echo "✅ User pre-provisioning without firebase_uid"
echo "✅ Email-based provisioning working"
echo ""
echo "📋 Next Step (Manual):"
echo "Have user with email $ADMIN_EMAIL login via Firebase"
echo "System will auto-link their firebase_uid to this account"
echo ""
