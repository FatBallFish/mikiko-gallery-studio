const puppeteer = require('puppeteer');

(async () => {
  const browser = await puppeteer.launch({ headless: 'new' });
  const page = await browser.newPage();
  await page.setViewport({ width: 2560, height: 1440 });
  
  // Test Home View
  await page.goto('http://localhost:5173/demo.html');
  await page.waitForTimeout(1000);
  
  // Click on the first image in the Home View masonry to trigger Lightbox
  await page.click('.columns-1 img');
  await page.waitForTimeout(500);
  
  await page.screenshot({ path: 'test_lightbox_home.png' });
  
  await browser.close();
})();
