package handler

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func isEntitlementAdmissionError(err error) bool {
	return errors.Is(err, service.ErrSubscriptionEntitlementQuotaExceeded) || errors.Is(err, service.ErrSubscriptionEntitlementNotFound) || errors.Is(err, service.ErrSubscriptionEntitlementExpired) || errors.Is(err, service.ErrSubscriptionEntitlementInactive) || errors.Is(err, service.ErrGroupNotAllowed) || errors.Is(err, service.ErrBillingServiceUnavailable) || errors.Is(err, service.ErrSubscriptionMaintenance)
}

// Run only after account selection and slot acquisition, immediately before forwarding.
// Release resources on rejection without reporting an upstream account failure.
func completeEntitlementBeforeForward(c *gin.Context, ent *service.SubscriptionEntitlement, release func(), writeError func(int, string, string)) bool {
	if ent == nil {
		return true
	}
	if err := service.CompleteEntitlementGenerationAdmission(c.Request.Context(), ent, 0); err != nil {
		if release != nil {
			release()
		}
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		writeError(status, code, message)
		return false
	}
	return true
}

func (h *OpenAIGatewayHandler) admitEntitlementBeforeForward(c *gin.Context, ent *service.SubscriptionEntitlement, release func(), streamStarted bool) bool {
	return completeEntitlementBeforeForward(c, ent, release, func(status int, code, message string) {
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
	})
}

func (h *GatewayHandler) admitEntitlementBeforeForward(c *gin.Context, ent *service.SubscriptionEntitlement, release func(), streamStarted bool) bool {
	return completeEntitlementBeforeForward(c, ent, release, func(status int, code, message string) {
		if strings.Contains(c.FullPath(), "/v1beta/") {
			if streamStarted {
				service.MarkOpsStreamError(c, code, message, status)
				c.SSEvent("error", gin.H{"error": gin.H{"code": status, "message": message, "status": googleapi.HTTPStatusToGoogleStatus(status)}})
				c.Writer.Flush()
			} else {
				googleError(c, status, message)
			}
			return
		}
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
	})
}
