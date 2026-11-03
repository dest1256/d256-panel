package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/outbound"

	"github.com/gin-gonic/gin"
)

// XraySettingController handles Xray configuration and settings operations.
type XraySettingController struct {
	XraySettingService          service.XraySettingService
	SettingService              service.SettingService
	InboundService              service.InboundService
	OutboundService             outbound.OutboundService
	XrayService                 service.XrayService
	OutboundSubscriptionService service.OutboundSubscriptionService
	GeodataService              service.GeodataService
}

// NewXraySettingController creates a new XraySettingController and initializes its routes.
func NewXraySettingController(g *gin.RouterGroup) *XraySettingController {
	a := &XraySettingController{}
	a.initRouter(g)
	return a
}

// initRouter sets up the routes for Xray settings management.
func (a *XraySettingController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/xray")
	g.GET("/getDefaultJsonConfig", a.getDefaultXrayConfig)

	g.POST("/", a.getXraySetting)
	g.POST("/update", a.updateSetting)

	g.GET("/geodata/files", a.geodataFiles)
	g.GET("/geodata/categories", a.geodataCategories)
	g.GET("/geodata/entries", a.geodataEntries)
	g.POST("/geodata/validate", a.geodataValidate)

	// Outbound subscription (remote outbound lists)
	g.GET("/outbound-subs", a.listOutboundSubs)
	g.POST("/outbound-subs", a.createOutboundSub)
	g.POST("/outbound-subs/:id/refresh", a.refreshOutboundSub)
	g.POST("/outbound-subs/:id/move", a.moveOutboundSub)
	g.POST("/outbound-subs/:id", a.updateOutboundSub)
	g.DELETE("/outbound-subs/:id", a.deleteOutboundSub)
	g.POST("/outbound-subs/:id/del", a.deleteOutboundSub) // POST alias for clients that can't send DELETE
	g.POST("/outbound-subs/parse", a.parseOutboundSubURL) // preview without saving
}

// getXraySetting retrieves the Xray configuration template, inbound tags, and outbound test URL.
func (a *XraySettingController) getXraySetting(c *gin.Context) {
	xraySetting, err := a.SettingService.GetXrayConfigTemplate()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	// Older versions of this handler embedded the raw DB value as
	// `xraySetting` in the response without checking if the value
	// already had that wrapper shape. When the frontend saved it
	// back through the textarea verbatim, the wrapper got persisted
	// and every subsequent save nested another layer, which is what
	// eventually produced the blank Xray Settings page in #4059.
	// Strip any such wrapper here, and heal the DB if we found one so
	// the next read is O(1) instead of climbing the same pile again.
	if unwrapped := service.UnwrapXrayTemplateConfig(xraySetting); unwrapped != xraySetting {
		if saveErr := a.XraySettingService.SaveXraySetting(unwrapped); saveErr == nil {
			xraySetting = unwrapped
		} else {
			// Don't fail the read — just serve the unwrapped value
			// and leave the DB healing for a later save.
			xraySetting = unwrapped
		}
	}
	inboundTags, err := a.InboundService.GetInboundTags()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	clientReverseTags, err := a.InboundService.GetClientReverseTags()
	if err != nil {
		clientReverseTags = "[]"
	}
	outboundTestUrl, _ := a.SettingService.GetXrayOutboundTestUrl()
	if outboundTestUrl == "" {
		outboundTestUrl = "https://www.google.com/generate_204"
	}
	xrayResponse := map[string]any{
		"xraySetting":       json.RawMessage(xraySetting),
		"inboundTags":       json.RawMessage(inboundTags),
		"clientReverseTags": json.RawMessage(clientReverseTags),
		"outboundTestUrl":   outboundTestUrl,
		"geodataSources":    service.StandardGeodataSources(),
	}

	// Surface subscription outbounds (and their tags) so the frontend can:
	// - show them as read-only items in the Outbounds tab
	// - let users pick them in balancers and routing rules
	// These are not part of the editable template; they are injected at runtime.
	if subObs, err := a.OutboundSubscriptionService.AllActiveOutbounds(); err == nil && len(subObs) > 0 {
		xrayResponse["subscriptionOutbounds"] = subObs
	}
	if subTags, err := a.OutboundSubscriptionService.AllActiveOutboundTags(); err == nil && len(subTags) > 0 {
		xrayResponse["subscriptionOutboundTags"] = subTags
	}
	result, err := json.Marshal(xrayResponse)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, string(result), nil)
}

// updateSetting updates the Xray configuration settings and applies them to
// the running core right away — through the gRPC API when only inbounds,
// outbounds or routing rules changed, with a process restart otherwise.
func (a *XraySettingController) updateSetting(c *gin.Context) {
	xraySetting := c.PostForm("xraySetting")
	if err := a.XraySettingService.SaveXraySetting(xraySetting); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	outboundTestUrl := c.PostForm("outboundTestUrl")
	if outboundTestUrl == "" {
		outboundTestUrl = "https://www.google.com/generate_204"
	}
	if err := a.SettingService.SetXrayOutboundTestUrl(outboundTestUrl); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	// Nodes pick the new template up through the desired-state push.
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), nil)
}

// getDefaultXrayConfig retrieves the default Xray configuration.
func (a *XraySettingController) getDefaultXrayConfig(c *gin.Context) {
	defaultJsonConfig, err := a.SettingService.GetDefaultXrayConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, defaultJsonConfig, nil)
}

// maxGeodataTokens bounds one validation request; a routing rule listing more
// categories than this is not something the panel needs to answer for.
const maxGeodataTokens = 500

// geodataFiles lists the geo databases Xray resolves geosite:/geoip: tokens
// against, including ones that failed to parse.
func (a *XraySettingController) geodataFiles(c *gin.Context) {
	files, err := a.GeodataService.Files()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, files, nil)
}

// geodataCategories returns one page of a database's categories.
func (a *XraySettingController) geodataCategories(c *gin.Context) {
	offset, limit := geodataPaging(c)
	page, err := a.GeodataService.Categories(c.Query("file"), c.Query("q"), offset, limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, page, nil)
}

// geodataEntries returns one page of the domains or CIDRs inside a category.
func (a *XraySettingController) geodataEntries(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("code is required"))
		return
	}
	offset, limit := geodataPaging(c)
	page, err := a.GeodataService.Entries(c.Query("file"), code, c.Query("q"), offset, limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, page, nil)
}

// geodataValidate reports which routing tokens do not resolve against the
// databases on disk.
func (a *XraySettingController) geodataValidate(c *gin.Context) {
	// Split with a bound rather than splitting first: a 10 MB body of commas
	// would otherwise allocate millions of strings before the limit is checked.
	tokens := strings.SplitN(c.PostForm("tokens"), ",", maxGeodataTokens+1)
	if len(tokens) > maxGeodataTokens {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewErrorf("too many tokens: over %d", maxGeodataTokens))
		return
	}
	jsonObj(c, a.GeodataService.Validate(c.PostForm("kind") == "ip", tokens), nil)
}

func geodataPaging(c *gin.Context) (int, int) {
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		offset = 0
	}
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 0
	}
	return offset, limit
}

// --- Outbound Subscription handlers ---

func (a *XraySettingController) listOutboundSubs(c *gin.Context) {
	list, err := a.OutboundSubscriptionService.List()
	if err != nil {
		jsonMsg(c, "Failed to list outbound subscriptions", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *XraySettingController) createOutboundSub(c *gin.Context) {
	remark := c.PostForm("remark")
	rawURL := c.PostForm("url")
	prefix := c.PostForm("tagPrefix")
	userAgent := c.PostForm("userAgent")
	enabled := c.PostForm("enabled") != "false"
	allowPrivate := c.PostForm("allowPrivate") == "true"
	allowInsecure := c.PostForm("allowInsecure") == "true"
	prepend := c.PostForm("prepend") == "true"
	intervalStr := c.PostForm("updateInterval")
	interval := 600
	if intervalStr != "" {
		if v, err := parseIntSafe(intervalStr); err == nil && v > 0 {
			interval = v
		}
	}
	sub, err := a.OutboundSubscriptionService.Create(remark, rawURL, prefix, userAgent, enabled, interval, allowPrivate, prepend, allowInsecure)
	if err != nil {
		jsonMsg(c, "Failed to create outbound subscription", err)
		return
	}
	jsonObj(c, sub, nil)
}

func (a *XraySettingController) updateOutboundSub(c *gin.Context) {
	id := c.Param("id")
	var subID int
	if _, err := fmt.Sscanf(id, "%d", &subID); err != nil {
		jsonMsg(c, "Invalid id", err)
		return
	}
	remark := c.PostForm("remark")
	rawURL := c.PostForm("url")
	prefix := c.PostForm("tagPrefix")
	userAgent := c.PostForm("userAgent")
	enabled := c.PostForm("enabled") != "false"
	allowPrivate := c.PostForm("allowPrivate") == "true"
	allowInsecure := c.PostForm("allowInsecure") == "true"
	prepend := c.PostForm("prepend") == "true"
	intervalStr := c.PostForm("updateInterval")
	interval := 600
	if intervalStr != "" {
		if v, err := parseIntSafe(intervalStr); err == nil && v > 0 {
			interval = v
		}
	}
	if err := a.OutboundSubscriptionService.Update(subID, remark, rawURL, prefix, userAgent, enabled, interval, allowPrivate, prepend, allowInsecure); err != nil {
		jsonMsg(c, "Failed to update outbound subscription", err)
		return
	}
	jsonObj(c, "", nil)
}

func (a *XraySettingController) deleteOutboundSub(c *gin.Context) {
	id := c.Param("id")
	var subID int
	if _, err := fmt.Sscanf(id, "%d", &subID); err != nil {
		jsonMsg(c, "Invalid id", err)
		return
	}
	if err := a.OutboundSubscriptionService.Delete(subID); err != nil {
		jsonMsg(c, "Failed to delete outbound subscription", err)
		return
	}
	// Signal that xray should drop this subscription's outbounds on next reload.
	a.XrayService.SetToNeedRestart()
	jsonObj(c, "", nil)
}

func (a *XraySettingController) refreshOutboundSub(c *gin.Context) {
	id := c.Param("id")
	var subID int
	if _, err := fmt.Sscanf(id, "%d", &subID); err != nil {
		jsonMsg(c, "Invalid id", err)
		return
	}
	obs, err := a.OutboundSubscriptionService.Refresh(subID)
	if err != nil {
		jsonMsg(c, "Refresh failed", err)
		return
	}
	// Signal that xray should pick up the new outbounds on next restart/reload
	a.XrayService.SetToNeedRestart()
	jsonObj(c, obs, nil)
}

func (a *XraySettingController) moveOutboundSub(c *gin.Context) {
	id := c.Param("id")
	var subID int
	if _, err := fmt.Sscanf(id, "%d", &subID); err != nil {
		jsonMsg(c, "Invalid id", err)
		return
	}
	up := c.PostForm("dir") == "up"
	if err := a.OutboundSubscriptionService.Move(subID, up); err != nil {
		jsonMsg(c, "Failed to reorder outbound subscription", err)
		return
	}
	// Order affects the merged outbounds, so xray needs a reload.
	a.XrayService.SetToNeedRestart()
	jsonObj(c, "", nil)
}

// parseOutboundSubURL is a preview endpoint: it fetches + parses the provided
// URL but does not persist anything. Useful for the "add subscription" flow
// so the user can see the resulting outbounds (and assigned tags) before saving.
func (a *XraySettingController) parseOutboundSubURL(c *gin.Context) {
	rawURL := c.PostForm("url")
	if rawURL == "" {
		jsonMsg(c, "url is required", common.NewError("missing url"))
		return
	}
	allowPrivate := c.PostForm("allowPrivate") == "true"
	allowInsecure := c.PostForm("allowInsecure") == "true"
	userAgent := c.PostForm("userAgent")
	// Use a throw-away service instance; it only needs the settingService for proxy.
	svc := service.OutboundSubscriptionService{}
	// We don't have a direct "fetch once" that returns without storing, so we
	// temporarily create a disabled row, refresh it, then delete. Cleaner would
	// be to expose a pure ParseURL on the service, but this keeps the surface small.
	tmp, err := svc.Create("preview", rawURL, "", userAgent, false, 600, allowPrivate, false, allowInsecure)
	if err != nil {
		jsonMsg(c, "Failed to preview subscription", err)
		return
	}
	obs, err := svc.Refresh(tmp.Id)
	// best-effort cleanup
	_ = svc.Delete(tmp.Id)
	if err != nil {
		jsonMsg(c, "Failed to fetch/parse subscription", err)
		return
	}
	jsonObj(c, obs, nil)
}

func parseIntSafe(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
