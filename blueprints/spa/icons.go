package spa

import (
	"slices"

	"github.com/patrickluzdev/esquadro/blueprints/ui"
)

// KitIcons are the icons the panel's kit draws, by their Lucide names: the
// set src/esquadro/icons.gen.ts exports, with those the model names, and
// src/icons.tsx may replace. A file of the kit that imports an icon from the
// set needs it here.
var KitIcons = []string{
	"Activity", "ArrowDown", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowUpDown",
	"Ban", "Banknote", "Bell", "BellOff", "Bold", "Bot",
	"Calendar", "ChartColumn", "Check", "CheckCheck", "ChevronDown", "ChevronLeft",
	"ChevronRight", "ChevronUp", "ChevronsUpDown", "Circle", "CircleAlert", "CircleCheck",
	"Code2", "Columns3", "Copy", "Download", "Eye", "EyeOff",
	"File", "FileQuestion", "FileUp", "Filter", "Hash", "Heading2",
	"Heading3", "History", "House", "ImagePlus", "Inbox", "Info",
	"Italic", "KeyRound", "LayoutDashboard", "Link2", "List", "ListChecks",
	"ListFilter", "ListOrdered", "Loader2", "LogOut", "Mail", "MailPlus",
	"Minus", "Monitor", "MonitorSmartphone", "Moon", "MoreHorizontal", "OctagonX",
	"Palette", "PanelLeft", "Pencil", "Plus", "Quote", "Redo2",
	"RefreshCw", "RotateCcw", "RotateCw", "Rows3", "Search", "SearchX",
	"Settings", "ShieldAlert", "ShieldCheck", "ShieldX", "Smartphone", "Star",
	"Strikethrough", "Sun", "Trash2", "TrendingDown", "TrendingUp", "TriangleAlert",
	"Underline", "Undo2", "Upload", "UserCheck", "UserPlus", "Users",
	"WifiOff", "X",
}

// WithIcon adds the Lucide name of an icon the model names (shopping_cart,
// or nothing for the default circle) to names.
func WithIcon(names []string, name any) []string {
	return append(names, ui.Lucide(name))
}

// IconSet is the kit's icons and those of names, sorted, each once.
func IconSet(names []string) []string {
	out := slices.Concat(KitIcons, names)
	slices.Sort(out)
	return slices.Compact(out)
}
