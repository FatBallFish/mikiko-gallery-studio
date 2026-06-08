import { applyJeePayWayCodeTemplate, jeepayWayCodeTemplates } from './cashierJeePayWayCodeTemplates'

const wxJSAPITemplate = jeepayWayCodeTemplates.find((item) => item.way_code === 'WX_JSAPI')
if (!wxJSAPITemplate || !wxJSAPITemplate.provider_types.includes('jeepay_wxpay')) {
  throw new Error(`JeePay WX_JSAPI template should be available for jeepay_wxpay, got ${JSON.stringify(wxJSAPITemplate)}`)
}

const merged = applyJeePayWayCodeTemplate(`{
  "gateway_url": "https://pay.example.com",
  "mch_no": "MCH10001",
  "app_id": "APP10001",
  "key": "merchant-secret",
  "client_ip": "127.0.0.1",
  "channel_extra": {
    "subAppId": "wx-sub-app"
  }
}`, 'WX_JSAPI')
const parsed = JSON.parse(merged)

if (parsed.gateway_url !== 'https://pay.example.com' || parsed.mch_no !== 'MCH10001' || parsed.key !== 'merchant-secret') {
  throw new Error(`JeePay template merge should preserve existing merchant config, got ${merged}`)
}

if (parsed.payment_mode !== 'api' || parsed.way_code !== 'WX_JSAPI') {
  throw new Error(`JeePay template should set API prepay mode and way_code, got ${merged}`)
}

if (parsed.channel_extra?.subAppId !== 'wx-sub-app' || !String(parsed.channel_extra?.openid).includes('openid')) {
  throw new Error(`JeePay template should merge channel_extra placeholders without dropping existing fields, got ${merged}`)
}

const wxH5 = applyJeePayWayCodeTemplate('{}', 'WX_H5')
if (!JSON.parse(wxH5).channel_extra?.sceneInfo) {
  throw new Error(`JeePay WX_H5 template should include sceneInfo placeholder, got ${wxH5}`)
}

const requiredTemplates = [
  ['WX_LITE', 'jeepay_wxpay'],
  ['WX_NATIVE_SUB_MCH', 'jeepay_wxpay'],
  ['WX_NATIVE_PROFIT_SHARING', 'jeepay_wxpay'],
  ['WX_H5_CATERING', 'jeepay_wxpay'],
  ['WX_NATIVE_PARKING', 'jeepay_wxpay'],
  ['ALI_JSAPI', 'jeepay_alipay'],
  ['ALI_PC_SUB_MCH', 'jeepay_alipay'],
  ['ALI_PC_HOTEL_PREAUTH', 'jeepay_alipay'],
  ['ALI_JSAPI_CAMPUS', 'jeepay_alipay'],
] as const

for (const [wayCode, providerType] of requiredTemplates) {
  const template = jeepayWayCodeTemplates.find((item) => item.way_code === wayCode)
  if (!template || !template.provider_types.includes(providerType)) {
    throw new Error(`JeePay template ${wayCode} should be available for ${providerType}, got ${JSON.stringify(template)}`)
  }
}

for (const template of jeepayWayCodeTemplates) {
  if (!template.category || !/[\u4e00-\u9fa5]/.test(template.category)) {
    throw new Error(`JeePay template ${template.way_code} should expose an operator-facing category, got ${JSON.stringify(template)}`)
  }
  const visibleCopy = `${template.category}${template.label}${template.description}`
  if (/占位|placeholder|后续|暂未|即将|版本/i.test(visibleCopy)) {
    throw new Error(`JeePay template visible copy should describe usable examples, got ${template.way_code}: ${visibleCopy}`)
  }
}

const wxLite = JSON.parse(applyJeePayWayCodeTemplate('{}', 'WX_LITE'))
if (wxLite.way_code !== 'WX_LITE' || wxLite.channel_extra?.openId !== '<mini-program-openid>' || wxLite.channel_extra?.appId !== '<mini-program-app-id>') {
  throw new Error(`JeePay WX_LITE template should include appId/openId placeholders, got ${JSON.stringify(wxLite)}`)
}

const wxSubMch = JSON.parse(applyJeePayWayCodeTemplate(`{
  "channel_extra": {
    "terminalInfo": "keep-existing"
  }
}`, 'WX_NATIVE_SUB_MCH'))
if (wxSubMch.channel_extra?.terminalInfo !== 'keep-existing' || wxSubMch.channel_extra?.subMchId !== '<sub-merchant-id>') {
  throw new Error(`JeePay sub merchant template should merge existing channel_extra and subMchId, got ${JSON.stringify(wxSubMch)}`)
}

const wxProfitSharing = JSON.parse(applyJeePayWayCodeTemplate('{}', 'WX_NATIVE_PROFIT_SHARING'))
if (!Array.isArray(wxProfitSharing.channel_extra?.profitSharingReceivers) || wxProfitSharing.channel_extra.profitSharingReceivers[0]?.account !== '<receiver-account>') {
  throw new Error(`JeePay profit sharing template should include receiver array placeholder, got ${JSON.stringify(wxProfitSharing)}`)
}

const aliJSAPI = JSON.parse(applyJeePayWayCodeTemplate('{}', 'ALI_JSAPI'))
if (aliJSAPI.channel_extra?.buyerUserId !== '<alipay-user-id>') {
  throw new Error(`JeePay ALI_JSAPI template should include buyerUserId placeholder, got ${JSON.stringify(aliJSAPI)}`)
}

const wxCatering = JSON.parse(applyJeePayWayCodeTemplate(`{
  "channel_extra": {
    "sceneInfo": {
      "wap_name": "Existing Shop"
    }
  }
}`, 'WX_H5_CATERING'))
if (wxCatering.way_code !== 'WX_H5' || wxCatering.channel_extra?.sceneInfo?.wap_name !== 'Existing Shop' || wxCatering.channel_extra?.storeInfo?.id !== '<store-id>' || wxCatering.channel_extra?.terminalInfo?.terminalId !== '<terminal-id>') {
  throw new Error(`JeePay catering template should merge scene/store/terminal industry parameters, got ${JSON.stringify(wxCatering)}`)
}

const wxParking = JSON.parse(applyJeePayWayCodeTemplate('{}', 'WX_NATIVE_PARKING'))
if (wxParking.way_code !== 'WX_NATIVE' || wxParking.channel_extra?.parkingInfo?.plateNumber !== '<plate-number>' || wxParking.channel_extra?.parkingInfo?.parkingId !== '<parking-id>') {
  throw new Error(`JeePay parking template should include parking industry parameters, got ${JSON.stringify(wxParking)}`)
}

const aliHotel = JSON.parse(applyJeePayWayCodeTemplate('{}', 'ALI_PC_HOTEL_PREAUTH'))
if (aliHotel.way_code !== 'ALI_PC' || aliHotel.channel_extra?.industryScenario !== 'HOTEL_PREAUTH' || aliHotel.channel_extra?.hotelOrderNo !== '<hotel-order-no>') {
  throw new Error(`JeePay hotel preauth template should include hotel industry parameters, got ${JSON.stringify(aliHotel)}`)
}

const aliCampus = JSON.parse(applyJeePayWayCodeTemplate(`{
  "channel_extra": {
    "buyerUserId": "existing-buyer"
  }
}`, 'ALI_JSAPI_CAMPUS'))
if (aliCampus.way_code !== 'ALI_JSAPI' || aliCampus.channel_extra?.buyerUserId !== 'existing-buyer' || aliCampus.channel_extra?.schoolInfo?.studentId !== '<student-id>') {
  throw new Error(`JeePay campus template should preserve buyerUserId and add school parameters, got ${JSON.stringify(aliCampus)}`)
}

const industryTemplates = jeepayWayCodeTemplates.filter((template) => template.category === '行业参数')
if (industryTemplates.length < 4 || !industryTemplates.some((template) => template.provider_types.includes('jeepay_alipay')) || !industryTemplates.some((template) => template.provider_types.includes('jeepay_wxpay'))) {
  throw new Error(`JeePay industry parameter templates should cover Alipay and WeChat scenarios, got ${JSON.stringify(industryTemplates)}`)
}
