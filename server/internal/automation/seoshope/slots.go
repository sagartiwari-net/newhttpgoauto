package seoshope

// Slot maps a panel task to the Semrush access button and upload targets.
type Slot struct {
	TaskUID   string
	ButtonNum string // span.semmy-btn-num text
	AccessKey string // fallback text match e.g. "access 2"
	WebsiteID string
	UploadURL string
}

// AllTaskUIDs lists every SEOShope automation for profile queue draining.
var AllTaskUIDs = []string{
	"seoshope_runSemrush",
	"seoshope_runSemrush2",
	"seoshope_runSemrush3",
	"seoshope_runSemrush4",
	"seoshope_runSemrush5",
	"seoshope_runSemrush6",
	"seoshope_runSemrush7",
	"seoshope_runSeoshopehome",
}

var slotsByTask = map[string]Slot{
	"seoshope_runSemrush":  {TaskUID: "seoshope_runSemrush", ButtonNum: "1", AccessKey: "access 1", WebsiteID: "seoshope", UploadURL: "https://refs.1clkaccess.store/shopesm1.php"},
	"seoshope_runSemrush2": {TaskUID: "seoshope_runSemrush2", ButtonNum: "2", AccessKey: "access 2", WebsiteID: "seoshope2", UploadURL: "https://refs.1clkaccess.store/shopesm2.php"},
	"seoshope_runSemrush3": {TaskUID: "seoshope_runSemrush3", ButtonNum: "3", AccessKey: "access 3", WebsiteID: "seoshope3", UploadURL: "https://refs.1clkaccess.store/shopesm3.php"},
	"seoshope_runSemrush4": {TaskUID: "seoshope_runSemrush4", ButtonNum: "4", AccessKey: "access 4", WebsiteID: "seoshope4", UploadURL: "https://refs.1clkaccess.store/shopesm4.php"},
	"seoshope_runSemrush5": {TaskUID: "seoshope_runSemrush5", ButtonNum: "5", AccessKey: "access 5", WebsiteID: "seoshope5", UploadURL: "https://refs.1clkaccess.store/shopesm5.php"},
	"seoshope_runSemrush6": {TaskUID: "seoshope_runSemrush6", ButtonNum: "6", AccessKey: "access 6", WebsiteID: "seoshope6", UploadURL: "https://refs.1clkaccess.store/shopesm6.php"},
	"seoshope_runSemrush7": {TaskUID: "seoshope_runSemrush7", ButtonNum: "8", AccessKey: "access 8", WebsiteID: "seoshope7", UploadURL: "https://refs.1clkaccess.store/shopesm7.php"},
}

func SlotForTask(taskUID string) (Slot, bool) {
	s, ok := slotsByTask[taskUID]
	return s, ok
}

func IsSeoshopeTask(taskUID string) bool {
	if taskUID == "seoshope_runSeoshopehome" {
		return true
	}
	_, ok := slotsByTask[taskUID]
	return ok
}
