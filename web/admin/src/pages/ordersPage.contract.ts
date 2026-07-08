import { OrdersPage } from './OrdersPage'
import { PackagesPage } from './PackagesPage'

if (typeof OrdersPage !== 'function') {
  throw new Error('OrdersPage should be exported as an independent admin page component')
}

if (typeof PackagesPage !== 'function') {
  throw new Error('PackagesPage should be exported as an independent admin page component')
}
