package wallet

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	_ "mu/internal/env" // load ~/.env before x402 config is read at init
)

// x402 configuration from the environment. Defaults target Base mainnet via
// Coinbase's hosted facilitator; set X402_FACILITATOR_URL to the open
// (testnet) facilitator to certify without real funds.
var (
	x402PayTo          = strings.TrimSpace(os.Getenv("X402_PAY_TO")) // receiving address
	x402FacilitatorURL = envOr("X402_FACILITATOR_URL", "https://api.cdp.coinbase.com/platform/v2/x402")
	// Advertised verbatim — the facilitator registers specific (network,
	// version) pairs. CDP settles the "exact" scheme as base+v1 or
	// eip155:8453+v2, so the defaults here (base, v1) match its v1 entry and
	// what common x402 clients speak. Set X402_NETWORK=eip155:8453 +
	// X402_VERSION=2 to use the v2 pair instead.
	x402NetworkID = envOr("X402_NETWORK", "base")
	x402Version   = envIntOr("X402_VERSION", 1) // advertised protocol version
)

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// normalizeNetwork accepts either CAIP-2 ids (eip155:8453) or the short v1
// names (base) and returns the CAIP-2 id, which the CDP facilitator uses.
func normalizeNetwork(n string) string {
	switch strings.ToLower(strings.TrimSpace(n)) {
	case "base", "eip155:8453":
		return "eip155:8453"
	case "base-sepolia", "eip155:84532":
		return "eip155:84532"
	default:
		return n
	}
}

// x402Asset is an ERC-20 accepted for payment on a given network. Name and
// Version are the token's EIP-712 domain parameters, echoed in a requirement's
// "extra" so the paying client can build the transfer-authorization signature.
type x402Asset struct {
	Symbol   string
	Address  string
	Decimals int
	Name     string
	Version  string
}

// x402AssetsByNetwork maps a network to its known stablecoins.
var x402AssetsByNetwork = map[string]map[string]x402Asset{
	"eip155:8453": { // Base mainnet
		"USDC": {"USDC", "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", 6, "USD Coin", "2"},
		"EURC": {"EURC", "0x60a3E35Cc302bFA44Cb288Bc5a4F316Fdb1adb42", 6, "EURC", "2"},
	},
	"eip155:84532": { // Base Sepolia
		"USDC": {"USDC", "0x036CbD53842c5426634e7929541eC2318f3dCF7e", 6, "USDC", "2"},
	},
}

// acceptedAssets returns the assets to advertise, honouring X402_ASSETS
// (comma-separated symbols) and defaulting to USDC only.
func acceptedAssets() []x402Asset {
	known := x402AssetsByNetwork[normalizeNetwork(x402NetworkID)]
	if known == nil {
		return nil
	}
	var out []x402Asset
	if list := strings.TrimSpace(os.Getenv("X402_ASSETS")); list != "" {
		for _, sym := range strings.Split(list, ",") {
			if a, ok := known[strings.ToUpper(strings.TrimSpace(sym))]; ok {
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 {
		if a, ok := known["USDC"]; ok {
			out = append(out, a)
		}
	}
	return out
}

// X402Enabled reports whether x402 payments are configured.
func X402Enabled() bool { return x402PayTo != "" }

// PaymentRequirements describes a remote server's x402 payment challenge for
// the outbound client. maxAmountRequired is in the asset's atomic units.
type PaymentRequirements struct {
	Scheme            string            `json:"scheme"`
	Network           string            `json:"network"`
	MaxAmountRequired string            `json:"maxAmountRequired"`
	Resource          string            `json:"resource"`
	Description       string            `json:"description"`
	MimeType          string            `json:"mimeType"`
	PayTo             string            `json:"payTo"`
	MaxTimeoutSeconds int               `json:"maxTimeoutSeconds"`
	Asset             string            `json:"asset"`
	Extra             map[string]string `json:"extra,omitempty"`
}

// settleRequirement verifies then settles a payment payload against a specific
// requirement (used when the amount isn't a fixed tool price, e.g. a USDC → credit
// top-up that sweeps an arbitrary balance).
func settleRequirement(payload map[string]any, req *PaymentRequirements) error {
	body := map[string]any{"x402Version": x402Version, "paymentPayload": payload, "paymentRequirements": req}

	vres, err := facilitatorPost("/verify", body)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	var verify struct {
		IsValid        bool   `json:"isValid"`
		Valid          bool   `json:"valid"`
		InvalidReason  string `json:"invalidReason"`
		InvalidMessage string `json:"invalidMessage"`
	}
	_ = json.Unmarshal(vres, &verify)
	if !verify.IsValid && !verify.Valid {
		return fmt.Errorf("payment invalid: %s", firstNonEmpty(verify.InvalidMessage, verify.InvalidReason, "rejected by facilitator"))
	}

	sres, err := facilitatorPost("/settle", body)
	if err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	var settle struct {
		Success     bool   `json:"success"`
		Transaction string `json:"transaction,omitempty"`
		Payer       string `json:"payer,omitempty"`
		ErrorReason string `json:"errorReason,omitempty"`
		Message     string `json:"message,omitempty"`
	}
	_ = json.Unmarshal(sres, &settle)
	if !settle.Success {
		return fmt.Errorf("settlement failed: %s", firstNonEmpty(settle.Message, settle.ErrorReason, "unknown"))
	}
	app.Log("x402", "settled %s: tx=%s payer=%s", req.Resource, settle.Transaction, settle.Payer)
	return nil
}

// usdcAtomicPerCredit is the 6-decimal USDC atomic amount for one credit
// (1 credit ≈ 1¢), so 1 USDC = 100 credits.
const usdcAtomicPerCredit = 10000

// ConvertUSDCToCredits sweeps the account's entire USDC balance to the treasury
// (gasless, settled via the facilitator) and credits the account at ~1¢/credit,
// so credits and crypto become one balance. Returns the credits added.
func ConvertUSDCToCredits(accountID string) (int, error) {
	if !X402Enabled() {
		return 0, fmt.Errorf("crypto payments are not configured")
	}
	bw := WalletFor(accountID)
	if bw == nil {
		return 0, fmt.Errorf("no wallet")
	}
	_, raw := USDCBalance(bw.Address)
	if raw == nil || raw.Sign() <= 0 {
		return 0, fmt.Errorf("no USDC balance to convert")
	}
	assets := acceptedAssets()
	if len(assets) == 0 {
		return 0, fmt.Errorf("no asset configured for this network")
	}
	credits := int(new(big.Int).Div(raw, big.NewInt(usdcAtomicPerCredit)).Int64())
	if credits <= 0 {
		return 0, fmt.Errorf("USDC amount too small to convert (min $0.01)")
	}

	a := assets[0]
	req := &PaymentRequirements{
		Scheme:            "exact",
		Network:           x402NetworkID,
		MaxAmountRequired: raw.String(),
		Resource:          "credit-topup",
		Description:       "USDC to credits",
		MimeType:          "application/json",
		PayTo:             x402PayTo,
		MaxTimeoutSeconds: 60,
		Asset:             a.Address,
		Extra:             map[string]string{"name": a.Name, "version": a.Version},
	}
	payloadB64, err := SignX402Payment(bw, *req)
	if err != nil {
		return 0, err
	}
	rawp, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		return 0, err
	}
	var payload map[string]any
	if err := json.Unmarshal(rawp, &payload); err != nil {
		return 0, err
	}
	if err := settleRequirement(payload, req); err != nil {
		return 0, err
	}
	if err := AddCredits(accountID, credits, "usdc_topup", map[string]interface{}{"usdc_atomic": raw.String()}); err != nil {
		return credits, err
	}
	app.Log("x402", "converted %s USDC atomic to %d credits for %s", raw.String(), credits, accountID)
	return credits, nil
}

// facilitatorPost POSTs JSON to a facilitator endpoint, attaching a CDP Bearer
// JWT when CDP credentials are configured (required by the Coinbase-hosted
// facilitator; ignored by the open one).
func facilitatorPost(path string, body map[string]any) ([]byte, error) {
	endpoint := strings.TrimRight(x402FacilitatorURL, "/") + path
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cdpConfigured() {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		bearer, err := cdpBearer(http.MethodPost, u.Host, u.Path)
		if err != nil {
			return nil, fmt.Errorf("cdp auth: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facilitator %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// X402Status returns a human-readable diagnostic of the x402 configuration
// and, when CDP credentials are present, the facilitator's advertised support —
// so an operator can certify auth on the box without exposing the secret.
func X402Status() string {
	var b strings.Builder
	fmt.Fprintf(&b, "enabled:       %v\n", X402Enabled())
	fmt.Fprintf(&b, "pay-to:        %s\n", firstNonEmpty(x402PayTo, "(X402_PAY_TO not set)"))
	fmt.Fprintf(&b, "facilitator:   %s\n", x402FacilitatorURL)
	fmt.Fprintf(&b, "network:       %s\n", x402NetworkID)
	fmt.Fprintf(&b, "version:       %d\n", x402Version)
	var syms []string
	for _, a := range acceptedAssets() {
		syms = append(syms, a.Symbol)
	}
	fmt.Fprintf(&b, "assets:        %s\n", firstNonEmpty(strings.Join(syms, ","), "(none for this network)"))
	fmt.Fprintf(&b, "cdp auth:      %v\n", cdpConfigured())

	if !cdpConfigured() {
		b.WriteString("\nNo CDP credentials (CDP_API_KEY_ID / CDP_API_KEY_SECRET). The open\nfacilitator only settles testnets; set CDP creds for Base mainnet.\n")
		return b.String()
	}

	endpoint := strings.TrimRight(x402FacilitatorURL, "/") + "/supported"
	u, err := url.Parse(endpoint)
	if err != nil {
		fmt.Fprintf(&b, "\ncdp probe:     bad facilitator URL: %v\n", err)
		return b.String()
	}
	bearer, err := cdpBearer(http.MethodGet, u.Host, u.Path)
	if err != nil {
		fmt.Fprintf(&b, "\ncdp probe:     JWT build failed: %v\n", err)
		return b.String()
	}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintf(&b, "\ncdp probe:     request failed: %v\n", err)
		return b.String()
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(&b, "\ncdp probe:     HTTP %d — auth NOT working: %s\n", resp.StatusCode, strings.TrimSpace(string(data)))
		return b.String()
	}
	fmt.Fprintf(&b, "\ncdp probe:     OK — auth working. Supported schemes/networks:\n%s\n", strings.TrimSpace(string(data)))
	if !strings.Contains(string(data), x402NetworkID) {
		fmt.Fprintf(&b, "\nWARNING: configured network %s not in the supported list above.\n", x402NetworkID)
	}
	return b.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
