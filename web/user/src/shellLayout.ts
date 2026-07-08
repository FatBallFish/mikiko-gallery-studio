import { cn } from '../../shared/classnames'
import { rdShell } from './ui/redesign-classes'

export type ShellScrollMode = 'app' | 'document'

export function shellLayoutClasses(mode: ShellScrollMode = 'app') {
  if (mode === 'document') {
    return {
      shell: cn(rdShell.shell, 'h-auto min-h-screen overflow-visible'),
      main: cn(rdShell.main, 'h-auto min-h-screen overflow-visible'),
    }
  }

  return {
    shell: rdShell.shell,
    main: rdShell.main,
  }
}
