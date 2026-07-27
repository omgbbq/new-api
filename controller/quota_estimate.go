package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

type QuotaEstimateRequest struct {
	Model            string  `json:"model" binding:"required"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Group            string  `json:"group"`
	RequestBody      *string `json:"request_body"`
}

type QuotaEstimateResponse struct {
	Model            string  `json:"model"`
	BillingMode      string  `json:"billing_mode"`
	Group            string  `json:"group"`
	GroupRatio       float64 `json:"group_ratio"`
	ModelRatio       float64 `json:"model_ratio,omitempty"`
	CompletionRatio  float64 `json:"completion_ratio,omitempty"`
	ModelPrice       float64 `json:"model_price,omitempty"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	QuotaBeforeGroup float64 `json:"quota_before_group"`
	Quota            int     `json:"quota"`
	EstimatedCost    float64 `json:"estimated_cost"`
	Currency         string  `json:"currency"`
	MatchedTier      string  `json:"matched_tier,omitempty"`
	Expression       string  `json:"expression,omitempty"`
}

func EstimateQuota(c *gin.Context) {
	var req QuotaEstimateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.PromptTokens < 0 || req.CompletionTokens < 0 {
		common.ApiErrorMsg(c, "token counts must be non-negative")
		return
	}

	groupRatio := 1.0
	group := req.Group
	if group != "" {
		groupRatio = ratio_setting.GetGroupRatio(group)
	} else {
		group = "default"
	}

	billingMode := billing_setting.GetBillingMode(req.Model)

	resp := QuotaEstimateResponse{
		Model:            req.Model,
		BillingMode:      billingMode,
		Group:            group,
		GroupRatio:       groupRatio,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
	}

	switch billingMode {
	case billing_setting.BillingModeTieredExpr:
		exprStr, ok := billing_setting.GetBillingExpr(req.Model)
		if !ok {
			common.ApiErrorMsg(c, "model is configured as tiered_expr but has no billing expression")
			return
		}

		var requestInput billingexpr.RequestInput
		if req.RequestBody != nil {
			requestInput.Body = []byte(*req.RequestBody)
		}

		rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
			P:   float64(req.PromptTokens),
			C:   float64(req.CompletionTokens),
			Len: float64(req.PromptTokens),
		}, requestInput)
		if err != nil {
			common.ApiErrorMsg(c, "expression evaluation failed: "+err.Error())
			return
		}

		quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
		quota := common.QuotaRound(quotaBeforeGroup * groupRatio)

		resp.Expression = exprStr
		resp.MatchedTier = trace.MatchedTier
		resp.QuotaBeforeGroup = quotaBeforeGroup
		resp.Quota = quota

	default:
		modelPrice, usePrice := ratio_setting.GetModelPrice(req.Model, false)
		if usePrice {
			n := 1
			if req.RequestBody != nil {
				if nVal := gjson.Get(*req.RequestBody, "n"); nVal.Exists() && nVal.Int() > 0 {
					n = int(nVal.Int())
				}
			}
			quotaBeforeGroup := modelPrice * float64(n) * common.QuotaPerUnit
			quota := common.QuotaRound(quotaBeforeGroup * groupRatio)

			resp.ModelPrice = modelPrice
			resp.QuotaBeforeGroup = quotaBeforeGroup
			resp.Quota = quota
		} else {
			modelRatio, _, _ := ratio_setting.GetModelRatio(req.Model)
			completionRatio := ratio_setting.GetCompletionRatio(req.Model)

			promptQuota := float64(req.PromptTokens)
			completionQuota := float64(req.CompletionTokens) * completionRatio
			quotaBeforeGroup := (promptQuota + completionQuota) * modelRatio
			quota := common.QuotaRound(quotaBeforeGroup * groupRatio)

			resp.ModelRatio = modelRatio
			resp.CompletionRatio = completionRatio
			resp.QuotaBeforeGroup = quotaBeforeGroup
			resp.Quota = quota
		}
	}

	displayType := operation_setting.GetQuotaDisplayType()
	switch displayType {
	case operation_setting.QuotaDisplayTypeCNY:
		resp.EstimatedCost = float64(resp.Quota) / common.QuotaPerUnit * operation_setting.USDExchangeRate
		resp.Currency = "CNY"
	case operation_setting.QuotaDisplayTypeCustom:
		resp.EstimatedCost = float64(resp.Quota) / common.QuotaPerUnit * operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
		resp.Currency = operation_setting.GetGeneralSetting().CustomCurrencySymbol
	default:
		resp.EstimatedCost = float64(resp.Quota) / common.QuotaPerUnit
		resp.Currency = "USD"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    resp,
	})
}
