import type { LucideIcon, LucideProps } from 'lucide-react'
import {
  Activity,
  BadgeAlert,
  Bell,
  ChevronDown,
  ChevronRight,
  Cloud,
  Coins,
  CreditCard,
  FileText,
  ImageOff,
  Inbox,
  Info,
  LayoutDashboard,
  LayoutPanelTop,
  LoaderCircle,
  Moon,
  NotebookText,
  Package,
  Route,
  Settings,
  ShieldCheck,
  Sun,
  Ticket,
  UserRound,
  Users,
  UsersRound,
} from 'lucide-react'

function withDefaults(Icon: LucideIcon) {
  return function AdminIcon({ strokeWidth = 1.5, ...props }: LucideProps) {
    return <Icon strokeWidth={strokeWidth} {...props} />
  }
}

export const DashboardIcon = withDefaults(LayoutDashboard)
export const MonitoringIcon = withDefaults(Activity)
export const UsersIcon = withDefaults(Users)
export const UserGroupsIcon = withDefaults(UsersRound)
export const CallRecordsIcon = withDefaults(NotebookText)
export const RedeemIcon = withDefaults(Ticket)
export const ReviewIcon = withDefaults(ShieldCheck)
export const OrdersIcon = withDefaults(CreditCard)
export const PackageIcon = withDefaults(Package)
export const CashierIcon = withDefaults(LayoutPanelTop)
export const RoutingIcon = withDefaults(Route)
export const AccessAccountsIcon = withDefaults(Cloud)
export const PricingIcon = withDefaults(Coins)
export const AuditIcon = withDefaults(FileText)
export const SystemUsersIcon = withDefaults(UserRound)
export const SystemSettingsIcon = withDefaults(Settings)

export const BellIcon = withDefaults(Bell)
export const SunIcon = withDefaults(Sun)
export const MoonIcon = withDefaults(Moon)
export const ChevronRightIcon = withDefaults(ChevronRight)
export const ChevronDownIcon = withDefaults(ChevronDown)
export const InfoIcon = withDefaults(Info)
export const EmptyIcon = withDefaults(Inbox)
export const ImageEmptyIcon = withDefaults(ImageOff)
export const AlertIcon = withDefaults(BadgeAlert)
export const LoaderIcon = withDefaults(LoaderCircle)
