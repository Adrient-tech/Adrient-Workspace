// Built-in icon set for Insert ▸ Icons (Material icons, rendered to PNG on insert).
import type { ComponentType } from "react";
import HomeIcon from "@mui/icons-material/Home";
import StarIcon from "@mui/icons-material/Star";
import FavoriteIcon from "@mui/icons-material/Favorite";
import CheckIcon from "@mui/icons-material/Check";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import CancelIcon from "@mui/icons-material/Cancel";
import InfoIcon from "@mui/icons-material/Info";
import WarningIcon from "@mui/icons-material/Warning";
import ErrorIcon from "@mui/icons-material/Error";
import HelpIcon from "@mui/icons-material/Help";
import LightbulbIcon from "@mui/icons-material/Lightbulb";
import RocketIcon from "@mui/icons-material/Rocket";
import SettingsIcon from "@mui/icons-material/Settings";
import PersonIcon from "@mui/icons-material/Person";
import PeopleIcon from "@mui/icons-material/People";
import GroupIcon from "@mui/icons-material/Group";
import BusinessIcon from "@mui/icons-material/Business";
import WorkIcon from "@mui/icons-material/Work";
import SchoolIcon from "@mui/icons-material/School";
import EmailIcon from "@mui/icons-material/Email";
import PhoneIcon from "@mui/icons-material/Phone";
import ChatIcon from "@mui/icons-material/Chat";
import CommentIcon from "@mui/icons-material/Comment";
import NotificationsIcon from "@mui/icons-material/Notifications";
import EventIcon from "@mui/icons-material/Event";
import ScheduleIcon from "@mui/icons-material/Schedule";
import TimerIcon from "@mui/icons-material/Timer";
import AlarmIcon from "@mui/icons-material/Alarm";
import PlaceIcon from "@mui/icons-material/Place";
import PublicIcon from "@mui/icons-material/Public";
import LanguageIcon from "@mui/icons-material/Language";
import CloudIcon from "@mui/icons-material/Cloud";
import StorageIcon from "@mui/icons-material/Storage";
import LockIcon from "@mui/icons-material/Lock";
import SecurityIcon from "@mui/icons-material/Security";
import ShieldIcon from "@mui/icons-material/Shield";
import KeyIcon from "@mui/icons-material/Key";
import VisibilityIcon from "@mui/icons-material/Visibility";
import SearchIcon from "@mui/icons-material/Search";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";
import RemoveIcon from "@mui/icons-material/Remove";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import TrendingUpIcon from "@mui/icons-material/TrendingUp";
import TrendingDownIcon from "@mui/icons-material/TrendingDown";
import BarChartIcon from "@mui/icons-material/BarChart";
import PieChartIcon from "@mui/icons-material/PieChart";
import ShowChartIcon from "@mui/icons-material/ShowChart";
import AnalyticsIcon from "@mui/icons-material/Analytics";
import AssessmentIcon from "@mui/icons-material/Assessment";
import DashboardIcon from "@mui/icons-material/Dashboard";
import DescriptionIcon from "@mui/icons-material/Description";
import FolderIcon from "@mui/icons-material/Folder";
import ArticleIcon from "@mui/icons-material/Article";
import ImageIcon from "@mui/icons-material/Image";
import CameraIcon from "@mui/icons-material/Camera";
import VideocamIcon from "@mui/icons-material/Videocam";
import MicIcon from "@mui/icons-material/Mic";
import HeadphonesIcon from "@mui/icons-material/Headphones";
import ShoppingCartIcon from "@mui/icons-material/ShoppingCart";
import StoreIcon from "@mui/icons-material/Store";
import PaymentsIcon from "@mui/icons-material/Payments";
import CreditCardIcon from "@mui/icons-material/CreditCard";
import AccountBalanceIcon from "@mui/icons-material/AccountBalance";
import SavingsIcon from "@mui/icons-material/Savings";
import ReceiptIcon from "@mui/icons-material/Receipt";
import InventoryIcon from "@mui/icons-material/Inventory";
import LocalShippingIcon from "@mui/icons-material/LocalShipping";
import FlightIcon from "@mui/icons-material/Flight";
import DirectionsCarIcon from "@mui/icons-material/DirectionsCar";
import BuildIcon from "@mui/icons-material/Build";
import CodeIcon from "@mui/icons-material/Code";
import TerminalIcon from "@mui/icons-material/Terminal";
import ExtensionIcon from "@mui/icons-material/Extension";
import MemoryIcon from "@mui/icons-material/Memory";
import WifiIcon from "@mui/icons-material/Wifi";
import BoltIcon from "@mui/icons-material/Bolt";
import RecyclingIcon from "@mui/icons-material/Recycling";
import SunnyIcon from "@mui/icons-material/Sunny";
import WaterIcon from "@mui/icons-material/Water";
import FlagIcon from "@mui/icons-material/Flag";
import BookmarkIcon from "@mui/icons-material/Bookmark";
import HandshakeIcon from "@mui/icons-material/Handshake";
import VerifiedIcon from "@mui/icons-material/Verified";
import GavelIcon from "@mui/icons-material/Gavel";
import EmojiEventsIcon from "@mui/icons-material/EmojiEvents";
import DiamondIcon from "@mui/icons-material/Diamond";
import SpeedIcon from "@mui/icons-material/Speed";
import AutorenewIcon from "@mui/icons-material/Autorenew";
import SyncIcon from "@mui/icons-material/Sync";
import LinkIcon from "@mui/icons-material/Link";
import ShareIcon from "@mui/icons-material/Share";
import DownloadIcon from "@mui/icons-material/Download";
import UploadIcon from "@mui/icons-material/Upload";
import PrintIcon from "@mui/icons-material/Print";
import CalculateIcon from "@mui/icons-material/Calculate";
import TranslateIcon from "@mui/icons-material/Translate";
import QuizIcon from "@mui/icons-material/Quiz";
import SupportIcon from "@mui/icons-material/Support";
import ScienceIcon from "@mui/icons-material/Science";
import MedicationIcon from "@mui/icons-material/Medication";
import RestaurantIcon from "@mui/icons-material/Restaurant";
import CoffeeIcon from "@mui/icons-material/Coffee";
import PetsIcon from "@mui/icons-material/Pets";

export interface IconEntry {
  name: string;
  label: string;
  Icon: ComponentType<{ htmlColor?: string }>;
}

export const ICON_LIBRARY: IconEntry[] = [
  { name: "Home", label: "Home", Icon: HomeIcon },
  { name: "Star", label: "Star", Icon: StarIcon },
  { name: "Favorite", label: "Favorite", Icon: FavoriteIcon },
  { name: "Check", label: "Check", Icon: CheckIcon },
  { name: "CheckCircle", label: "Check Circle", Icon: CheckCircleIcon },
  { name: "Cancel", label: "Cancel", Icon: CancelIcon },
  { name: "Info", label: "Info", Icon: InfoIcon },
  { name: "Warning", label: "Warning", Icon: WarningIcon },
  { name: "Error", label: "Error", Icon: ErrorIcon },
  { name: "Help", label: "Help", Icon: HelpIcon },
  { name: "Lightbulb", label: "Lightbulb", Icon: LightbulbIcon },
  { name: "Rocket", label: "Rocket", Icon: RocketIcon },
  { name: "Settings", label: "Settings", Icon: SettingsIcon },
  { name: "Person", label: "Person", Icon: PersonIcon },
  { name: "People", label: "People", Icon: PeopleIcon },
  { name: "Group", label: "Group", Icon: GroupIcon },
  { name: "Business", label: "Business", Icon: BusinessIcon },
  { name: "Work", label: "Work", Icon: WorkIcon },
  { name: "School", label: "School", Icon: SchoolIcon },
  { name: "Email", label: "Email", Icon: EmailIcon },
  { name: "Phone", label: "Phone", Icon: PhoneIcon },
  { name: "Chat", label: "Chat", Icon: ChatIcon },
  { name: "Comment", label: "Comment", Icon: CommentIcon },
  { name: "Notifications", label: "Notifications", Icon: NotificationsIcon },
  { name: "Event", label: "Event", Icon: EventIcon },
  { name: "Schedule", label: "Schedule", Icon: ScheduleIcon },
  { name: "Timer", label: "Timer", Icon: TimerIcon },
  { name: "Alarm", label: "Alarm", Icon: AlarmIcon },
  { name: "Place", label: "Place", Icon: PlaceIcon },
  { name: "Public", label: "Public", Icon: PublicIcon },
  { name: "Language", label: "Language", Icon: LanguageIcon },
  { name: "Cloud", label: "Cloud", Icon: CloudIcon },
  { name: "Storage", label: "Storage", Icon: StorageIcon },
  { name: "Lock", label: "Lock", Icon: LockIcon },
  { name: "Security", label: "Security", Icon: SecurityIcon },
  { name: "Shield", label: "Shield", Icon: ShieldIcon },
  { name: "Key", label: "Key", Icon: KeyIcon },
  { name: "Visibility", label: "Visibility", Icon: VisibilityIcon },
  { name: "Search", label: "Search", Icon: SearchIcon },
  { name: "Edit", label: "Edit", Icon: EditIcon },
  { name: "Delete", label: "Delete", Icon: DeleteIcon },
  { name: "Add", label: "Add", Icon: AddIcon },
  { name: "Remove", label: "Remove", Icon: RemoveIcon },
  { name: "ArrowForward", label: "Arrow Forward", Icon: ArrowForwardIcon },
  { name: "ArrowBack", label: "Arrow Back", Icon: ArrowBackIcon },
  { name: "ArrowUpward", label: "Arrow Upward", Icon: ArrowUpwardIcon },
  { name: "ArrowDownward", label: "Arrow Downward", Icon: ArrowDownwardIcon },
  { name: "TrendingUp", label: "Trending Up", Icon: TrendingUpIcon },
  { name: "TrendingDown", label: "Trending Down", Icon: TrendingDownIcon },
  { name: "BarChart", label: "Bar Chart", Icon: BarChartIcon },
  { name: "PieChart", label: "Pie Chart", Icon: PieChartIcon },
  { name: "ShowChart", label: "Show Chart", Icon: ShowChartIcon },
  { name: "Analytics", label: "Analytics", Icon: AnalyticsIcon },
  { name: "Assessment", label: "Assessment", Icon: AssessmentIcon },
  { name: "Dashboard", label: "Dashboard", Icon: DashboardIcon },
  { name: "Description", label: "Description", Icon: DescriptionIcon },
  { name: "Folder", label: "Folder", Icon: FolderIcon },
  { name: "Article", label: "Article", Icon: ArticleIcon },
  { name: "Image", label: "Image", Icon: ImageIcon },
  { name: "Camera", label: "Camera", Icon: CameraIcon },
  { name: "Videocam", label: "Videocam", Icon: VideocamIcon },
  { name: "Mic", label: "Mic", Icon: MicIcon },
  { name: "Headphones", label: "Headphones", Icon: HeadphonesIcon },
  { name: "ShoppingCart", label: "Shopping Cart", Icon: ShoppingCartIcon },
  { name: "Store", label: "Store", Icon: StoreIcon },
  { name: "Payments", label: "Payments", Icon: PaymentsIcon },
  { name: "CreditCard", label: "Credit Card", Icon: CreditCardIcon },
  { name: "AccountBalance", label: "Account Balance", Icon: AccountBalanceIcon },
  { name: "Savings", label: "Savings", Icon: SavingsIcon },
  { name: "Receipt", label: "Receipt", Icon: ReceiptIcon },
  { name: "Inventory", label: "Inventory", Icon: InventoryIcon },
  { name: "LocalShipping", label: "Local Shipping", Icon: LocalShippingIcon },
  { name: "Flight", label: "Flight", Icon: FlightIcon },
  { name: "DirectionsCar", label: "Directions Car", Icon: DirectionsCarIcon },
  { name: "Build", label: "Build", Icon: BuildIcon },
  { name: "Code", label: "Code", Icon: CodeIcon },
  { name: "Terminal", label: "Terminal", Icon: TerminalIcon },
  { name: "Extension", label: "Extension", Icon: ExtensionIcon },
  { name: "Memory", label: "Memory", Icon: MemoryIcon },
  { name: "Wifi", label: "Wifi", Icon: WifiIcon },
  { name: "Bolt", label: "Bolt", Icon: BoltIcon },
  { name: "Recycling", label: "Recycling", Icon: RecyclingIcon },
  { name: "Sunny", label: "Sunny", Icon: SunnyIcon },
  { name: "Water", label: "Water", Icon: WaterIcon },
  { name: "Flag", label: "Flag", Icon: FlagIcon },
  { name: "Bookmark", label: "Bookmark", Icon: BookmarkIcon },
  { name: "Handshake", label: "Handshake", Icon: HandshakeIcon },
  { name: "Verified", label: "Verified", Icon: VerifiedIcon },
  { name: "Gavel", label: "Gavel", Icon: GavelIcon },
  { name: "EmojiEvents", label: "Emoji Events", Icon: EmojiEventsIcon },
  { name: "Diamond", label: "Diamond", Icon: DiamondIcon },
  { name: "Speed", label: "Speed", Icon: SpeedIcon },
  { name: "Autorenew", label: "Autorenew", Icon: AutorenewIcon },
  { name: "Sync", label: "Sync", Icon: SyncIcon },
  { name: "Link", label: "Link", Icon: LinkIcon },
  { name: "Share", label: "Share", Icon: ShareIcon },
  { name: "Download", label: "Download", Icon: DownloadIcon },
  { name: "Upload", label: "Upload", Icon: UploadIcon },
  { name: "Print", label: "Print", Icon: PrintIcon },
  { name: "Calculate", label: "Calculate", Icon: CalculateIcon },
  { name: "Translate", label: "Translate", Icon: TranslateIcon },
  { name: "Quiz", label: "Quiz", Icon: QuizIcon },
  { name: "Support", label: "Support", Icon: SupportIcon },
  { name: "Science", label: "Science", Icon: ScienceIcon },
  { name: "Medication", label: "Medication", Icon: MedicationIcon },
  { name: "Restaurant", label: "Restaurant", Icon: RestaurantIcon },
  { name: "Coffee", label: "Coffee", Icon: CoffeeIcon },
  { name: "Pets", label: "Pets", Icon: PetsIcon },
];

/** Entries whose name or label contains every word of `q`. */
export function searchIcons(q: string, lib: IconEntry[] = ICON_LIBRARY): IconEntry[] {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return lib;
  return lib.filter((i) => words.every((w) => i.label.toLowerCase().includes(w) || i.name.toLowerCase().includes(w)));
}

/** An SVG document for icon markup (an `<svg>` string) in `color`. */
export function iconSvgDocument(markup: string, color: string, px = 256): string {
  const safe = /^#[0-9a-f]{3,8}$/i.test(color) ? color : "#000000";
  const inner = markup.replace(/^<svg[^>]*>/, "").replace(/<\/svg>$/, "");
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${px}" height="${px}" viewBox="0 0 24 24" fill="${safe}">${inner}</svg>`;
}
