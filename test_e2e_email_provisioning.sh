#!/bin/bash

# Complete End-to-End Email Provisioning Test
# 1. Super admin creates tenant and pre-provisions tenant_admin
# 2. Tenant admin logs in via Firebase
# 3. Verify firebase_uid was linked and user has correct data

set -e

API_URL="http://localhost:8080"
FIREBASE_API_KEY="AIzaSyBSLjVinF_VdJOQwtQxNqD7TVBj0wCJR60"

echo "🧪 Complete Email Provisioning E2E Test"
echo "================================================"
echo ""

# ============================================
# PHASE 1: Super Admin Provisions User
# ============================================
echo "📋 PHASE 1: Super Admin Provisioning"
echo "============================================"

# Step 1: Get Firebase token for super_admin (test@homgym.app)
echo "📝 Step 1: Getting super_admin Firebase token..."
SUPER_FIREBASE_OUTPUT=$(python3 get_firebase_token.py 2>&1)
SUPER_FIREBASE=$(echo "$SUPER_FIREBASE_OUTPUT" | grep -A 1 "Your ID Token" | tail -1 | tr -d ' ')

if [ -z "$SUPER_FIREBASE" ] || [[ ! "$SUPER_FIREBASE" =~ ^eyJ ]]; then
    echo "❌ Failed to get super_admin Firebase token"
    exit 1
fi
echo "✅ Super admin Firebase token obtained"

# Step 2: Login as super_admin
echo "📝 Step 2: Login as super_admin..."
SUPER_ADMIN_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Authorization: Bearer $SUPER_FIREBASE" \
  -H "Content-Type: application/json" \
  -d '{"requested_role": "super_admin"}')

SUPER_ADMIN_TOKEN=$(echo $SUPER_ADMIN_RESPONSE | jq -r '.token')
echo "✅ Logged in as super_admin"

# Step 3: Create HOM Fitness tenant
echo "📝 Step 3: Creating tenant..."
TENANT_RESPONSE=$(curl -s -X POST "$API_URL/v1/platform/tenants" \
  -H "Authorization: Bearer $SUPER_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "HOM Fitness E2E Test",
    "join_code": "HOM-E2E-'$(date +%s)'",
    "logo_url": "https://homfitness.com/logo.png"
  }')

TENANT_ID=$(echo $TENANT_RESPONSE | jq -r '.id')
TENANT_NAME=$(echo $TENANT_RESPONSE | jq -r '.name')
echo "✅ Tenant created: $TENANT_NAME (ID: $TENANT_ID)"

# Step 4: Pre-provision tenant_admin (EMAIL ONLY)
echo "📝 Step 4: Pre-provisioning tenant_admin..."
ADMIN_EMAIL="test-admin-$(date +%s)@homgym.app"
ADMIN_PASSWORD="123456"

ADMIN_CREATE_RESPONSE=$(curl -s -X POST "$API_URL/v1/platform/users" \
  -H "Authorization: Bearer $SUPER_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "'$ADMIN_EMAIL'",
    "name": "Adam (E2E Test)",
    "roles": ["tenant_admin"],
    "tenant_id": "'$TENANT_ID'"
  }')

ADMIN_USER_ID=$(echo $ADMIN_CREATE_RESPONSE | jq -r '.id')
ADMIN_PRE_FIREBASE_UID=$(echo $ADMIN_CREATE_RESPONSE | jq -r '.firebase_uid')

if [ "$ADMIN_USER_ID" = "null" ]; then
    echo "❌ User creation failed!"
    echo "Response: $ADMIN_CREATE_RESPONSE"
    exit 1
fi

echo "✅ Tenant admin pre-provisioned"
echo "   User ID: $ADMIN_USER_ID"
echo "   Email: $ADMIN_EMAIL"
echo "   Firebase UID (before): $ADMIN_PRE_FIREBASE_UID (should be empty)"
echo ""

# Step 5: Get Firebase token for the pre-provisioned user (creates account if needed)
echo "📝 Step 5: Getting Firebase token for $ADMIN_EMAIL..."
FIREBASE_TOKEN=$(python3 firebase_signin.py "$ADMIN_EMAIL" "$ADMIN_PASSWORD" 2>&1)

if [ -z "$FIREBASE_TOKEN" ] || [[ ! "$FIREBASE_TOKEN" =~ ^eyJ ]]; then
    echo "❌ Failed to get Firebase token"
    echo "Output: $FIREBASE_TOKEN"
    exit 1
fi

# Extract Firebase UID from token (decode JWT)
FIREBASE_UID=$(echo $FIREBASE_TOKEN | cut -d'.' -f2 | base64 -d 2>/dev/null | jq -r '.user_id')

echo "✅ Firebase token obtained"
echo "   Firebase UID: $FIREBASE_UID"
echo ""

# ============================================
# PHASE 2: Tenant Admin First Login
# ============================================
echo "📋 PHASE 2: Tenant Admin First Login (Account Linking)"
echo "============================================"

# Step 6: Tenant admin logs in (should auto-link)
echo "📝 Step 6: Tenant admin logging in..."
ADMIN_LOGIN_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Authorization: Bearer $FIREBASE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requested_role": "tenant_admin"}')

ADMIN_TOKEN=$(echo $ADMIN_LOGIN_RESPONSE | jq -r '.token')
ADMIN_LINKED_ID=$(echo $ADMIN_LOGIN_RESPONSE | jq -r '.user.id')
ADMIN_LINKED_FIREBASE_UID=$(echo $ADMIN_LOGIN_RESPONSE | jq -r '.user.firebase_uid')
ADMIN_LINKED_TENANT_ID=$(echo $ADMIN_LOGIN_RESPONSE | jq -r '.user.tenant_id')
ADMIN_LINKED_ROLES=$(echo $ADMIN_LOGIN_RESPONSE | jq -r '.user.roles | join(", ")')
IS_NEW_USER=$(echo $ADMIN_LOGIN_RESPONSE | jq -r '.is_new_user')

echo "✅ Tenant admin logged in"
echo ""

# ============================================
# PHASE 3: Verification
# ============================================
echo "📋 PHASE 3: Verification"
echo "============================================"

# Check 1: User ID should match
echo "🔍 Check 1: User ID matches"
if [ "$ADMIN_USER_ID" = "$ADMIN_LINKED_ID" ]; then
    echo "   ✅ PASS: User ID matches ($ADMIN_USER_ID)"
else
    echo "   ❌ FAIL: User ID mismatch!"
    echo "      Expected: $ADMIN_USER_ID"
    echo "      Got: $ADMIN_LINKED_ID"
    exit 1
fi

# Check 2: Firebase UID should now be linked
echo "🔍 Check 2: Firebase UID linked"
if [ "$ADMIN_LINKED_FIREBASE_UID" = "$FIREBASE_UID" ] && [ -n "$ADMIN_LINKED_FIREBASE_UID" ]; then
    echo "   ✅ PASS: Firebase UID linked ($FIREBASE_UID)"
else
    echo "   ❌ FAIL: Firebase UID not linked!"
    echo "      Expected: $FIREBASE_UID"
    echo "      Got: $ADMIN_LINKED_FIREBASE_UID"
    exit 1
fi

# Check 3: Tenant ID should match
echo "🔍 Check 3: Tenant ID correct"
if [ "$ADMIN_LINKED_TENANT_ID" = "$TENANT_ID" ]; then
    echo "   ✅ PASS: Tenant ID matches ($TENANT_ID)"
else
    echo "   ❌ FAIL: Tenant ID mismatch!"
    echo "      Expected: $TENANT_ID"
    echo "      Got: $ADMIN_LINKED_TENANT_ID"
    exit 1
fi

# Check 4: Role should be tenant_admin
echo "🔍 Check 4: Role is tenant_admin"
if [[ "$ADMIN_LINKED_ROLES" == *"tenant_admin"* ]]; then
    echo "   ✅ PASS: Has tenant_admin role"
else
    echo "   ❌ FAIL: Missing tenant_admin role!"
    echo "      Got: $ADMIN_LINKED_ROLES"
    exit 1
fi

# Check 5: Should NOT be a new user
echo "🔍 Check 5: Not flagged as new user"
if [ "$IS_NEW_USER" = "false" ]; then
    echo "   ✅ PASS: Correctly identified as existing user"
else
    echo "   ❌ FAIL: Flagged as new user (should be false)"
    echo "      Got: $IS_NEW_USER"
    exit 1
fi

echo ""
echo "================================================"
echo "🎉 ALL TESTS PASSED!"
echo "================================================"
echo ""
echo "Summary:"
echo "  • Email: $ADMIN_EMAIL"
echo "  • User ID: $ADMIN_USER_ID"
echo "  • Firebase UID: $FIREBASE_UID ✅ LINKED"
echo "  • Tenant: $TENANT_NAME ($TENANT_ID)"
echo "  • Role: tenant_admin"
echo ""
echo "✅ Email-based provisioning working perfectly!"
echo "✅ Account auto-linking successful!"
echo ""
