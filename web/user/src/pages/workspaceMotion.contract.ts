import { readFileSync } from 'node:fs'
import { rdWorkspace, state } from '../ui/redesign-classes'

const workspaceSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

const workspaceMotionClasses = {
  generatedImage: /generatedImage:\s*'([^']+)'/.exec(workspaceSource)?.[1] ?? '',
  historyImage: /historyImage:\s*'([^']+)'/.exec(workspaceSource)?.[1] ?? '',
  historyDialogImage: /historyDialogImage:\s*'([^']+)'/.exec(workspaceSource)?.[1] ?? '',
  outputResultWrap: /outputResultWrap:\s*'([^']+)'/.exec(workspaceSource)?.[1] ?? '',
  slotSkeletonGlow: /slotSkeletonGlow:\s*'([^']+)'/.exec(workspaceSource)?.[1] ?? '',
  refRemove: /refRemove:\s*'([^']+)'/.exec(workspaceSource)?.[1] ?? '',
}

assertContainsAll('generated image scale transition', workspaceMotionClasses.generatedImage, [
  'group-hover:scale-[1.04]',
  'motion-reduce:transition-none',
  'motion-reduce:group-hover:scale-100',
])
assertContainsAll('history image scale transition', workspaceMotionClasses.historyImage, [
  'group-hover:scale-105',
  'motion-reduce:transition-none',
  'motion-reduce:group-hover:scale-100',
])
assertContainsAll('history dialog image scale transition', workspaceMotionClasses.historyDialogImage, [
  'group-hover:scale-[1.03]',
  'motion-reduce:transition-none',
  'motion-reduce:group-hover:scale-100',
])
assertContainsAll('result entrance', workspaceMotionClasses.outputResultWrap, [
  'animate-in',
  'zoom-in-95',
  'motion-reduce:animate-none',
])
assertContainsAll('workspace skeleton shimmer', workspaceMotionClasses.slotSkeletonGlow, [
  'animate-[shimmer_1.8s_linear_infinite]',
  'motion-reduce:animate-none',
])
assertContainsAll('reference remove affordance', workspaceMotionClasses.refRemove, [
  'group-focus-within:opacity-100',
  'focus-visible:opacity-100',
  'max-[760px]:opacity-100',
  '[@media(pointer:coarse)]:opacity-100',
])
assertContainsAll('generate button shimmer', rdWorkspace.btnGlow, [
  'group-hover:animate-[shimmer_2s_infinite]',
  'motion-reduce:animate-none',
])
assertContainsAll('outer workspace loader', rdWorkspace.outputRingInner1, ['animate-ping', 'motion-reduce:animate-none'])
assertContainsAll('inner workspace loader', rdWorkspace.outputRingInner2, ['animate-pulse', 'motion-reduce:animate-none'])
assertContainsAll('reused spinner', state.spinner, ['animate-spin', 'motion-reduce:animate-none'])
assertContainsAll('reused result entrance', rdWorkspace.outputImageWrap, ['animate-in', 'zoom-in-95', 'motion-reduce:animate-none'])

function assertContainsAll(label: string, className: string, expected: string[]) {
  for (const token of expected) {
    if (!className.includes(token)) {
      throw new Error(`${label} must include ${token}, got ${className}`)
    }
  }
}
