import { focusTrapTargetIndex } from './focusTrap'

if (focusTrapTargetIndex(-1, 3, false) !== 0) {
  throw new Error('Tab from the dialog container should enter the first focusable control')
}
if (focusTrapTargetIndex(-1, 3, true) !== 2) {
  throw new Error('Shift+Tab from the dialog container should enter the last focusable control')
}
if (focusTrapTargetIndex(2, 3, false) !== 0) {
  throw new Error('Tab should wrap from the last control to the first')
}
if (focusTrapTargetIndex(0, 3, true) !== 2) {
  throw new Error('Shift+Tab should wrap from the first control to the last')
}
if (focusTrapTargetIndex(1, 3, false) !== null || focusTrapTargetIndex(1, 3, true) !== null) {
  throw new Error('focus should follow native order away from trap boundaries')
}
if (focusTrapTargetIndex(-1, 0, false) !== null) {
  throw new Error('an empty dialog should keep focus on its container')
}
