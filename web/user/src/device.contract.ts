import fs from 'node:fs'
import { isMobileDeviceUserAgent } from './device'

const source = fs.readFileSync(new URL('./device.ts', import.meta.url), 'utf8')
if (!source.includes('navigator.userAgent')) {
  throw new Error('isMobileDeviceUA must inspect navigator.userAgent')
}

const mobileUserAgents = [
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1',
  'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36',
  'Mozilla/5.0 (iPad; CPU OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1',
  'Mozilla/5.0 (Linux; HarmonyOS; HBD-AL00; HMSCore 6.6.0.312) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.88 HuaweiBrowser/12.0.4.303 Mobile Safari/537.36',
  'Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.42',
  'Mozilla/5.0 (Linux; U; Android 13; zh-CN; M2012K11AC Build/TKQ1.220829.002) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0.4896.58 UCBrowser/15.5.4.1239 Mobile Safari/537.36 AliApp(AP/10.5.66.8000)',
  'Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 AlipayClient/10.5.66',
]
for (const userAgent of mobileUserAgents) {
  if (!isMobileDeviceUserAgent(userAgent)) {
    throw new Error(`mobile user agent must be detected: ${userAgent}`)
  }
}

const desktopUserAgents = [
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:127.0) Gecko/20100101 Firefox/127.0',
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
]
for (const userAgent of desktopUserAgents) {
  if (isMobileDeviceUserAgent(userAgent)) {
    throw new Error(`desktop user agent must not be detected as mobile: ${userAgent}`)
  }
}
