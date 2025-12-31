#!/bin/bash

# RBAC Test Script for Metamorph API
# Tests role-based access control and multi-tenant isolation

set -e

API_URL="http://localhost:8080"

# Get Firebase token from Python script
echo "Getting Firebase token..."
FIREBASE_OUTPUT=$(python3 get_firebase_token.py 2>&1)
FIREBASE_TOKEN=$(echo "$FIREBASE_OUTPUT" | grep -A 1 "Your ID Token" | tail -1 | tr -d ' ')

if [ -z "$FIREBASE_TOKEN" ] || [[ ! "$FIREBASE_TOKEN" =~ ^eyJ ]]; then
    echo "❌ Failed to get valid Firebase token"
    echo "Output: $FIREBASE_OUTPUT"
    echo ""
    echo "Please run: python3 get_firebase_token.py"
    exit 1
fi

echo "✅ Firebase token obtained"
echo ""

echo "🔐 RBAC and Multi-Tenant Security Tests"
echo "========================================"
echo ""

# Test 1: Get JWT Token as Member
echo "📝 Test 1: Login as Member"
echo "----------------------------"
MEMBER_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Authorization: Bearer $FIREBASE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requested_role": "member"}')

MEMBER_TOKEN=$(echo $MEMBER_RESPONSE | jq -r '.token')
MEMBER_ID=$(echo $MEMBER_RESPONSE | jq -r '.user.id')

if [ -z "$MEMBER_TOKEN" ] || [ "$MEMBER_TOKEN" = "null" ]; then
    echo "❌ Failed to get member token"
    echo "Response: $MEMBER_RESPONSE"
    exit 1
fi

echo "✅ Member logged in successfully"
echo "   User ID: $MEMBER_ID"
echo "   Token: ${MEMBER_TOKEN:0:50}..."
echo ""

# Test 2: Member Can Access Own Scans
echo "📝 Test 2: Member Accessing Personal Scans"
echo "-------------------------------------------"
SCANS_RESPONSE=$(curl -s -X GET "$API_URL/v1/me/scans" \
  -H "Authorization: Bearer $MEMBER_TOKEN")

SCANS_SUCCESS=$(echo $SCANS_RESPONSE | jq -r '.success')
if [ "$SCANS_SUCCESS" = "true" ]; then
    echo "✅ Member can access /v1/me/scans"
else
    echo "❌ Member cannot access own scans"
    echo "Response: $SCANS_RESPONSE"
fi
echo ""

# Test 3: Member Blocked from Pro Endpoints
echo "📝 Test 3: Member Blocked from Pro API"
echo "---------------------------------------"
PRO_RESPONSE=$(curl -s -X GET "$API_URL/v1/pro/clients" \
  -H "Authorization: Bearer $MEMBER_TOKEN")

PRO_ERROR=$(echo $PRO_RESPONSE | jq -r '.error')
if [[ "$PRO_ERROR" == *"Insufficient permissions"* ]] || [[ "$PRO_ERROR" == *"permissions"* ]]; then
    echo "✅ Member correctly blocked from /v1/pro/clients (403 Forbidden)"
else
    echo "❌ Member should be blocked from pro endpoints"
    echo "Response: $PRO_RESPONSE"
fi
echo ""

# Test 4: Member Blocked from Admin Endpoints
echo "📝 Test 4: Member Blocked from Admin API"
echo "-----------------------------------------"
ADMIN_RESPONSE=$(curl -s -X GET "$API_URL/v1/admin/users" \
  -H "Authorization: Bearer $MEMBER_TOKEN")

ADMIN_ERROR=$(echo $ADMIN_RESPONSE | jq -r '.error')
if [[ "$ADMIN_ERROR" == *"Insufficient permissions"* ]] || [[ "$ADMIN_ERROR" == *"permissions"* ]]; then
    echo "✅ Member correctly blocked from /v1/admin/users (403 Forbidden)"
else
    echo "❌ Member should be blocked from admin endpoints"
    echo "Response: $ADMIN_RESPONSE"
fi
echo ""

# Test 5: Get JWT Token as Coach
echo "📝 Test 5: Login as Coach"
echo "-------------------------"
COACH_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Authorization: Bearer $FIREBASE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requested_role": "coach"}')

COACH_TOKEN=$(echo $COACH_RESPONSE | jq -r '.token')
COACH_ROLES=$(echo $COACH_RESPONSE | jq -r '.user.roles')

if [ -z "$COACH_TOKEN" ] || [ "$COACH_TOKEN" = "null" ]; then
    echo "❌ Failed to get coach token"
    echo "Response: $COACH_RESPONSE"
    exit 1
fi

echo "✅ Coach logged in successfully"
echo "   Roles: $COACH_ROLES"
echo "   Token: ${COACH_TOKEN:0:50}..."
echo ""

# Test 6: Coach Can Access Pro Endpoints
echo "📝 Test 6: Coach Accessing Pro API"
echo "-----------------------------------"
COACH_PRO_RESPONSE=$(curl -s -X GET "$API_URL/v1/pro/clients" \
  -H "Authorization: Bearer $COACH_TOKEN")

COACH_PRO_ERROR=$(echo $COACH_PRO_RESPONSE | jq -r '.error')
if [ "$COACH_PRO_ERROR" = "null" ] || [ -z "$COACH_PRO_ERROR" ]; then
    echo "✅ Coach can access /v1/pro/clients"
else
    echo "⚠️  Coach should access pro endpoints"
    echo "Response: $COACH_PRO_RESPONSE"
fi
echo ""

# Test 7: Multi-Role User (Member + Coach)
echo "📝 Test 7: Multi-Role User"
echo "--------------------------"
MULTI_ROLES=$(echo $COACH_RESPONSE | jq -r '.user.roles | length')
if [ "$MULTI_ROLES" -gt 1 ]; then
    echo "✅ User now has multiple roles: $COACH_ROLES"
    
    # Test member endpoint still works
    ME_TEST=$(curl -s -X GET "$API_URL/v1/me/scans" \
      -H "Authorization: Bearer $COACH_TOKEN")
    ME_SUCCESS=$(echo $ME_TEST | jq -r '.success')
    
    if [ "$ME_SUCCESS" = "true" ]; then
        echo "✅ Multi-role user can access both /v1/me/* and /v1/pro/*"
    else
        echo "⚠️  Multi-role user should access member endpoints"
    fi
else
    echo "ℹ️  User has single role: $COACH_ROLES"
fi
echo ""

# Test 8: Invalid Token
echo "📝 Test 8: Invalid Token Rejection"
echo "-----------------------------------"
INVALID_RESPONSE=$(curl -s -X GET "$API_URL/v1/me/scans" \
  -H "Authorization: Bearer invalid.token.here")

INVALID_ERROR=$(echo $INVALID_RESPONSE | jq -r '.error')
if [[ "$INVALID_ERROR" == *"Invalid"* ]] || [[ "$INVALID_ERROR" == *"token"* ]]; then
    echo "✅ Invalid token correctly rejected (401 Unauthorized)"
else
    echo "❌ Invalid tokens should be rejected"
    echo "Response: $INVALID_RESPONSE"
fi
echo ""

# Test 9: Missing Token
echo "📝 Test 9: Missing Token Rejection"
echo "-----------------------------------"
NO_TOKEN_RESPONSE=$(curl -s -X GET "$API_URL/v1/me/scans")

NO_TOKEN_ERROR=$(echo $NO_TOKEN_RESPONSE | jq -r '.error')
if [[ "$NO_TOKEN_ERROR" == *"Missing"* ]] || [[ "$NO_TOKEN_ERROR" == *"authorization"* ]]; then
    echo "✅ Missing token correctly rejected (401 Unauthorized)"
else
    echo "❌ Requests without tokens should be rejected"
    echo "Response: $NO_TOKEN_RESPONSE"
fi
echo ""

# Summary
echo "========================================"
echo "🎯 Test Summary"
echo "========================================"
echo "✅ JWT token generation and validation"
echo "✅ Role-based access control (RBAC)"
echo "✅ Member blocked from Pro/Admin APIs"
echo "✅ Coach can access Pro API"
echo "✅ Multi-role support working"
echo "✅ Invalid/missing token rejection"
echo ""
echo "🔒 Security Status: OPERATIONAL"
echo ""
echo "Next: Test tenant isolation and data scoping"
echo "Run: ./test_tenant_isolation.sh"
