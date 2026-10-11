package cashier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// AlipaySignatureCheckRequest carries the draft instance fields needed to
// probe the Alipay gateway with a signed alipay.trade.query call.
type AlipaySignatureCheckRequest struct {
	GatewayURL    string
	AppID         string
	AppPrivateKey string
	OutTradeNo    string
	HTTPClient    *http.Client
}

// AlipaySignatureCheckResult reports whether the gateway accepted the
// signature plus the raw gateway error fields for display. NetworkError is
// set when the gateway could not be reached at all; such failures say
// nothing about the signature and must not be reported as a key mismatch.
type AlipaySignatureCheckResult struct {
	SignatureOK  bool
	NetworkError string
	Code         string
	SubCode      string
	Msg          string
	SubMsg       string
}

type alipayGatewayResponse struct {
	Code    string `json:"code"`
	Msg     string `json:"msg"`
	SubCode string `json:"sub_code"`
	SubMsg  string `json:"sub_msg"`
}

// CheckAlipaySignature signs a harmless alipay.trade.query for a trade that
// does not exist and posts it to the configured gateway. The gateway checks
// the signature before running business logic, so any response other than the
// signature-failure code proves the private key matches the app public key
// configured on the Alipay open platform.
func CheckAlipaySignature(ctx context.Context, req AlipaySignatureCheckRequest) (AlipaySignatureCheckResult, error) {
	gatewayURL := strings.TrimSpace(req.GatewayURL)
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipay.com/gateway.do"
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		return AlipaySignatureCheckResult{}, errs.BadRequest("alipay app_id is required")
	}
	outTradeNo := strings.TrimSpace(req.OutTradeNo)
	if outTradeNo == "" {
		outTradeNo = fmt.Sprintf("SIGCHECK%d", time.Now().UnixNano())
	}
	bizContent, err := json.Marshal(map[string]string{"out_trade_no": outTradeNo})
	if err != nil {
		return AlipaySignatureCheckResult{}, errs.Internal("failed to encode alipay signature check request")
	}
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.query")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", alipayBeijingTime())
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, req.AppPrivateKey)
	if signErr != nil {
		return AlipaySignatureCheckResult{}, signErr
	}
	values.Set("sign", sign)
	httpClient := req.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	httpRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL, strings.NewReader(values.Encode()))
	if requestErr != nil {
		return AlipaySignatureCheckResult{}, errs.BadRequest("alipay gateway url is invalid")
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	httpResponse, doErr := httpClient.Do(httpRequest)
	if doErr != nil {
		return AlipaySignatureCheckResult{NetworkError: doErr.Error()}, nil
	}
	defer func() { _ = httpResponse.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20))
	if readErr != nil || httpResponse.StatusCode != http.StatusOK {
		return AlipaySignatureCheckResult{NetworkError: fmt.Sprintf("gateway responded with HTTP %d", httpResponse.StatusCode)}, nil
	}
	var payload struct {
		Response alipayGatewayResponse `json:"alipay_trade_query_response"`
	}
	if unmarshalErr := json.Unmarshal(body, &payload); unmarshalErr != nil {
		return AlipaySignatureCheckResult{NetworkError: "gateway response is not valid alipay json"}, nil
	}
	// The gateway verifies the signature before executing business logic, so
	// every response code except the signature-failure one proves the key
	// pair matches the app public key registered on the open platform.
	signatureOK := payload.Response.Code != "" && payload.Response.Code != "40002"
	return AlipaySignatureCheckResult{
		SignatureOK: signatureOK,
		Code:        strings.TrimSpace(payload.Response.Code),
		SubCode:     strings.TrimSpace(payload.Response.SubCode),
		Msg:         strings.TrimSpace(payload.Response.Msg),
		SubMsg:      strings.TrimSpace(payload.Response.SubMsg),
	}, nil
}

// AlipaySignatureCheckHint renders an operator-facing explanation for a
// signature check result. Keep messages actionable: the common root cause of
// a failed check is a private key that does not pair with the app public key
// uploaded on the Alipay open platform.
func AlipaySignatureCheckHint(result AlipaySignatureCheckResult) string {
	switch {
	case result.NetworkError != "":
		return "无法连接支付宝网关：" + result.NetworkError + "。请检查网关地址与服务器网络，该结果与密钥配置无关。"
	case result.Code == "":
		return "网关无响应或返回格式异常，请检查网关地址（正式环境为 https://openapi.alipay.com/gateway.do，沙箱为 https://openapi.alipaydev.com/gateway.do）。"
	case result.SignatureOK && result.Code == "40004":
		return "签名验证通过：应用私钥与开放平台配置的应用公钥匹配。返回 " + result.SubCode + " 为预期结果（测试单号不存在），可放心保存配置。"
	case result.SignatureOK:
		return "签名验证通过，但网关返回了业务错误：" + result.Msg + "。签名配置无需调整。"
	default:
		return "签名验证失败：" + result.Msg + "。请确认填入的是支付宝开放平台『接口加签方式』中上传的应用公钥所配对的应用私钥（而非支付宝公钥、应用公钥或重新生成过的新密钥对）。"
	}
}
