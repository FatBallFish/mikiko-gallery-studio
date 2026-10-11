// Coarse device detection used to pick the payment redirect UX
// (new window vs. same-page navigation). It intentionally stays a
// user-agent heuristic: the backend already picked page.pay / wap.pay
// from the request UA, the frontend only needs the fallback flavor.
const mobileDeviceUAPattern = /android|iphone|ipod|ipad|mobile|phone|harmonyos|micromessenger|aliapp|alipayclient/i

export function isMobileDeviceUserAgent(userAgent: string): boolean {
  return mobileDeviceUAPattern.test(userAgent)
}

export function isMobileDeviceUA(): boolean {
  return isMobileDeviceUserAgent(navigator.userAgent)
}
