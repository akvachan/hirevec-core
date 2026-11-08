// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/bits"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/akvachan/hirevec-core"
)

func main() {
	out := flag.String("out", "data/evaluation_dataset/1", "output directory for the dataset")
	seed := flag.Int64("seed", 42, "RNG seed (deterministic output per seed)")
	nPositions := flag.Int("positions", 30000, "number of positions")
	nCandidates := flag.Int("candidates", 100, "number of candidates")
	top := flag.Int("top", 10, "top-N positions per candidate recommendation")
	flag.Parse()

	slog.SetLogLoggerLevel(slog.LevelError)
	hirevec.InitLogger(
		hirevec.MustParseEnv("HIREVEC_LOG_LEVEL", hirevec.ParseLogLevel, slog.LevelInfo),
		hirevec.MustParseEnv("HIREVEC_LOG_JSON", strconv.ParseBool, false),
	)

	start := time.Now()

	if *nPositions < 1 || *nCandidates < 1 || *top < 1 {
		slog.Error("invalid flags", "positions", *nPositions, "candidates", *nCandidates, "top", *top)
		panic("positions, candidates and top must all be >= 1")
	}
	slog.Info(
		"generating dataset",
		"out", *out,
		"seed", *seed,
		"positions", *nPositions,
		"candidates", *nCandidates,
		"top", *top,
	)

	rng := rand.New(rand.NewSource(*seed))

	stage := time.Now()
	positions := GeneratePositions(rng, *nPositions)
	slog.Info("positions generated", "count", len(positions), "duration", time.Since(stage))

	stage = time.Now()
	WritePositionsJSONL(positions, filepath.Join(*out, "positions.jsonl"))
	slog.Info("positions written", "path", filepath.Join(*out, "positions.jsonl"), "duration", time.Since(stage))

	stage = time.Now()
	candidates := GenerateCandidates(rng, *nCandidates)
	slog.Info("candidates generated", "count", len(candidates), "duration", time.Since(stage))

	stage = time.Now()
	WriteCandidatesJSONL(candidates, filepath.Join(*out, "candidates.jsonl"))
	slog.Info("candidates written", "path", filepath.Join(*out, "candidates.jsonl"), "duration", time.Since(stage))

	stage = time.Now()
	recommendations := GenerateRecommendations(rng, positions, candidates, *top)
	slog.Info("recommendations generated", "count", len(recommendations), "duration", time.Since(stage))

	stage = time.Now()
	WriteRecommendationsJSONL(recommendations, filepath.Join(*out, "recommendations.jsonl"))
	slog.Info("recommendations written", "path", filepath.Join(*out, "recommendations.jsonl"), "duration", time.Since(stage))

	stage = time.Now()
	WriteJSON(filepath.Join(*out, "meta.json"), Meta{
		Description: "Low-quality, high-variety dataset for basic recommendation engine tests and evaluations.",
		Methods:     []GenerationMethod{{Type: "synthetic-template", Stage: 0, Model: ""}},
	})
	slog.Debug("meta written", "path", filepath.Join(*out, "meta.json"), "duration", time.Since(stage))

	stage = time.Now()
	if err := Validate(*out, *nPositions, *nCandidates); err != nil {
		slog.Error("validation failed", "err", err)
		panic(fmt.Errorf("failed to validate: %w", err))
	}
	slog.Info("validate finished", "duration", time.Since(stage))

	slog.Info("dataset generation complete", "duration", time.Since(start).Round(time.Millisecond), "out", *out)
}

type Position struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Skill1      string `json:"skill_1"`
	Skill2      string `json:"skill_2"`
	Skill3      string `json:"skill_3"`
	Skill4      string `json:"skill_4"`
	Skill5      string `json:"skill_5"`
	ID          int    `json:"position_id"`
	IsRemote    int    `json:"is_remote"`
	DomainIndex int    `json:"-"`
}

func WritePositionsJSONL(positions []Position, path string) {
	slog.Debug("writing positions", "path", path, "count", len(positions))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Error("failed to create output dir", "path", filepath.Dir(path), "err", err)
		panic(err)
	}
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	for _, r := range positions {
		if err := encoder.Encode(r); err != nil {
			panic(err)
		}
	}
	if err := writer.Flush(); err != nil {
		panic(err)
	}
}

type CandidateExperience struct {
	Title          string `json:"title"`
	StartedAt      string `json:"started_at"`
	EndedAt        string `json:"ended_at"`
	Description    string `json:"description"`
	Company        string `json:"company"`
	ExperienceType string `json:"experience_type"`
	Skill1         string `json:"skill_1"`
	Skill2         string `json:"skill_2"`
	Skill3         string `json:"skill_3"`
	Skill4         string `json:"skill_4"`
	Skill5         string `json:"skill_5"`
}

type Candidate struct {
	CandidateExperiences []CandidateExperience `json:"candidate_experiences"`
	SkillSet             map[string]bool       `json:"-"`
	About                string                `json:"about"`
	MostRecentTitle      string                `json:"-"`
	MostRecentCompany    string                `json:"-"`
	PrimaryIndex         int                   `json:"-"`
	SecondaryIndex       int                   `json:"-"`
	ID                   int                   `json:"candidate_id"`
}

func WriteCandidatesJSONL(candidates []Candidate, path string) {
	slog.Debug("writing candidates", "path", path, "count", len(candidates))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Error("failed to create output dir", "path", filepath.Dir(path), "err", err)
		panic(err)
	}
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)

	for _, r := range candidates {
		if err := encoder.Encode(r); err != nil {
			panic(err)
		}
	}
}

type RankedPosition struct {
	Reasoning  string  `json:"reasoning"`
	Score      float64 `json:"score"`
	Rank       int     `json:"rank"`
	PositionID int     `json:"position_id"`
}

type Recommendation struct {
	Positions   []RankedPosition `json:"positions"`
	CandidateID int              `json:"candidate_id"`
	ID          int              `json:"id"`
}

func WriteRecommendationsJSONL(recommendations []Recommendation, path string) {
	slog.Debug("writing recommendations", "path", path, "count", len(recommendations))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Error("failed to create output dir", "path", filepath.Dir(path), "err", err)
		panic(err)
	}
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)

	for _, r := range recommendations {
		if err := encoder.Encode(r); err != nil {
			panic(err)
		}
	}
}

type GenerationMethod struct {
	Type  string `json:"type"`
	Model string `json:"model,omitempty"`
	Stage int    `json:"stage"`
}

type Meta struct {
	Methods     []GenerationMethod `json:"methods"`
	Description string             `json:"description"`
}

type Domain struct {
	Companies     []string
	Titles        []string
	ManagerTitles []string
	Skills        []string
	Education     []string
	Certs         []string
	Tasks         []string
	Name          string
	RemoteChance  float64
}

var Domains = []Domain{
	{
		Name:          "Beauty & Wellness",
		Companies:     []string{"Belle Epoque Salon", "Glam Studio", "Urban Cuts", "Serenity Day Spa", "The Barber Corner", "Aurora Beauty Lounge"},
		Titles:        []string{"Hairdresser", "Barber", "Beautician", "Nail Technician", "Makeup Artist"},
		ManagerTitles: []string{"Salon Manager", "Spa Manager"},
		Skills:        []string{"Hair Cutting", "Hair Coloring", "Barbering", "Beard Trimming", "Manicure", "Pedicure", "Waxing", "Eyelash Extensions", "Facial Treatments", "Client Consultation"},
		Education:     []string{"Vocational Programme in Hairdressing", "Diploma in Cosmetology"},
		Certs:         []string{"First Aid Certificate", "Hygiene & Safety Certificate"},
		Tasks: []string{
			"perform haircuts, coloring and styling for a loyal client base",
			"carry out manicure, pedicure and waxing treatments",
			"consult with clients and recommend suitable treatments",
			"keep workstations clean and sanitized throughout the day",
			"manage bookings and maintain accurate client records",
		},
		RemoteChance: 0.02,
	},
	{
		Name:          "Transport & Logistics",
		Companies:     []string{"Cardinal Freight", "SwiftHaul Logistics", "Baltic Distribution", "RouteNine Cargo", "Delta Movers", "Transfield Logistics"},
		Titles:        []string{"Truck Driver", "Delivery Driver", "Warehouse Worker", "Forklift Operator", "Dispatcher", "Logistics Coordinator"},
		ManagerTitles: []string{"Warehouse Manager", "Depot Manager"},
		Skills:        []string{"CDL Class A", "Long-Haul Driving", "Forklift Operation", "Route Planning", "Load Securing", "Inventory Management", "Freight Documentation", "Warehouse Management Systems", "Safety Compliance"},
		Education:     []string{"Vocational Programme in Logistics", "High School Diploma"},
		Certs:         []string{"Forklift License", "Dangerous Goods Handling Certificate", "First Aid Certificate"},
		Tasks: []string{
			"transport goods between regional distribution hubs",
			"load and unload deliveries while keeping accurate documentation",
			"pick, pack and palletize incoming and outgoing freight",
			"plan daily routes and coordinate drivers in real time",
			"keep the warehouse organized and inventory accurate",
		},
		RemoteChance: 0.03,
	},
	{
		Name:          "Construction & Trades",
		Companies:     []string{"Nordwall Construction", "Sterling & Sons Builders", "Ironpeak Contractors", "BuildRight Group", "Atlas Industrial Contracting", "Stonebridge Works"},
		Titles:        []string{"Electrician", "Plumber", "Welder", "Carpenter", "Bricklayer", "Scaffolder", "HVAC Technician"},
		ManagerTitles: []string{"Site Manager", "Construction Supervisor"},
		Skills:        []string{"Electrical Installation", "Pipefitting", "MIG Welding", "Blueprint Reading", "Carpentry", "Bricklaying", "Scaffolding", "HVAC Maintenance", "Concrete Forming", "Site Safety"},
		Education:     []string{"Vocational School, Electrical Installation", "Apprenticeship in Plumbing"},
		Certs:         []string{"Working at Heights Certificate", "Site Safety Card", "First Aid Certificate"},
		Tasks: []string{
			"install, maintain and repair systems on residential and commercial sites",
			"read blueprints and technical drawings",
			"work to tight schedules on active construction sites",
			"perform installations in line with safety regulations",
			"coordinate with other trades to keep the project on track",
		},
		RemoteChance: 0.02,
	},
	{
		Name:          "Hospitality & Food",
		Companies:     []string{"The Copper Table", "Grand Meridien Hotel", "Bistro Amaretto", "Harbor House Restaurant", "The Daily Grind Coffee", "Sunset Beach Resort"},
		Titles:        []string{"Cook", "Chef de Partie", "Waiter", "Bartender", "Barista", "Hotel Receptionist", "Housekeeper"},
		ManagerTitles: []string{"Kitchen Manager", "Restaurant Manager"},
		Skills:        []string{"Hot Kitchen Preparation", "Grill and Frying", "Pastry Basics", "Espresso Brewing", "Cocktail Mixing", "Fine Dining Service", "Guest Relations", "HACCP", "Food Safety", "Cash Handling"},
		Education:     []string{"Culinary School Diploma", "Hotel Management Programme"},
		Certs:         []string{"Food Hygiene Certificate", "HACCP Certificate", "First Aid Certificate"},
		Tasks: []string{
			"prepare dishes to order in a busy kitchen",
			"serve guests and handle orders and payments",
			"prepare espresso drinks exactly to standard",
			"keep food safety and cleanliness standards at all times",
			"welcome guests and resolve day-to-day issues on the floor",
		},
		RemoteChance: 0.02,
	},
	{
		Name:          "Retail & Consumer",
		Companies:     []string{"MegaMart", "Cornerstone Grocery", "Fashion Barn", "FreshDaily Markets", "UrbanThings Electronics", "GreenLeaf Pharmacy Retail"},
		Titles:        []string{"Sales Assistant", "Cashier", "Visual Merchandiser", "Stock Clerk", "Pharmacy Assistant"},
		ManagerTitles: []string{"Store Manager", "Branch Manager"},
		Skills:        []string{"Cash Handling", "POS Systems", "Merchandising", "Upselling", "Inventory Control", "Customer Service", "Product Knowledge", "Complaint Handling"},
		Education:     []string{"High School Diploma", "Vocational Programme in Retail"},
		Certs:         []string{"First Aid Certificate", "Cash Handling Training"},
		Tasks: []string{
			"advise customers and drive daily sales targets",
			"operate the register and handle cash accurately",
			"maintain shelf displays and planogram compliance",
			"receive stock and keep the backroom organized",
			"handle customer questions and returns professionally",
		},
		RemoteChance: 0.05,
	},
	{
		Name:          "Healthcare & Care",
		Companies:     []string{"St. Aldric Regional Hospital", "GreenMeadows Care Home", "Downtown Medical Center", "Rosewood Elderly Care", "BrightPath Clinic", "Harborview Health Group"},
		Titles:        []string{"Registered Nurse", "Caregiver", "Nursing Assistant", "Medical Receptionist", "Physiotherapy Assistant", "Home Care Worker"},
		ManagerTitles: []string{"Nursing Home Manager", "Ward Manager"},
		Skills:        []string{"Patient Care", "Medication Administration", "Wound Care", "Vital Signs Monitoring", "Elderly Care", "Medical Documentation", "Phlebotomy", "Patient Transport", "Empathy and Communication"},
		Education:     []string{"Bachelor of Science in Nursing", "Care Assistant Vocational Programme"},
		Certs:         []string{"First Aid Certificate", "Medication Administration Training", "Hygiene Certificate"},
		Tasks: []string{
			"provide day-to-day care and monitor patient well-being",
			"administer medication according to care plans",
			"document care activities accurately and on time",
			"support patients with mobility and personal care",
			"work closely with nurses, physicians and families",
		},
		RemoteChance: 0.03,
	},
	{
		Name:          "Office & Administration",
		Companies:     []string{"Meridian Consulting", "Aldergate Business Services", "Cobalt Property Group", "Vertex Staffing", "Northline Insurance", "Harmonia Media"},
		Titles:        []string{"Administrative Assistant", "Receptionist", "Office Assistant", "HR Assistant", "Payroll Clerk", "Personal Assistant"},
		ManagerTitles: []string{"Office Manager", "Head of Administration"},
		Skills:        []string{"Calendar Management", "MS Excel", "Word Processing", "Data Entry", "Invoicing", "Payroll Administration", "Recruitment Support", "Correspondence", "Meeting Organization"},
		Education:     []string{"Bachelor of Business Administration", "Vocational Programme in Office Administration"},
		Certs:         []string{"MS Office Specialist", "First Aid Certificate"},
		Tasks: []string{
			"manage calendars, travel bookings and meeting logistics",
			"prepare documents, reports and correspondence",
			"keep records, files and shared drives up to date",
			"support payroll and HR processes",
			"greet visitors and handle front-desk communication",
		},
		RemoteChance: 0.35,
	},
	{
		Name:          "Accounting & Finance",
		Companies:     []string{"Fairborn & Partners", "LedgerLine Accounting", "Quayside Financial", "Summit Audit Group", "BrightBooks Accounting", "Ravenscourt Advisory"},
		Titles:        []string{"Accountant", "Bookkeeper", "Accounts Payable Clerk", "Accounts Receivable Clerk", "Payroll Specialist", "Financial Assistant"},
		ManagerTitles: []string{"Finance Manager", "Head of Accounting"},
		Skills:        []string{"Double-Entry Bookkeeping", "Ledger Reconciliation", "Accounts Payable", "Accounts Receivable", "VAT Returns", "Payroll Accounting", "Financial Reporting", "MS Excel", "ERP Systems"},
		Education:     []string{"Bachelor of Accounting", "Bachelor of Finance", "Vocational Programme in Bookkeeping"},
		Certs:         []string{"Certified Bookkeeper", "Payroll Compliance Training"},
		Tasks: []string{
			"post journal entries and reconcile ledgers",
			"process supplier invoices and employee expenses",
			"prepare VAT statements and month-end closings",
			"run payroll cycles for assigned clients",
			"support audits with accurate documentation",
		},
		RemoteChance: 0.30,
	},
	{
		Name:          "Education & Childcare",
		Companies:     []string{"Riverdale Primary School", "Little Steps Kindergarten", "Greenfield International School", "Bright Minds Tutoring", "Sunnyhill Preschool", "Oakwood Secondary School"},
		Titles:        []string{"Teacher", "Kindergarten Teacher", "Teaching Assistant", "Tutor", "School Secretary", "Special Education Assistant"},
		ManagerTitles: []string{"School Principal", "Head of Studies"},
		Skills:        []string{"Lesson Planning", "Classroom Management", "Child Development", "Early Childhood Education", "Differentiated Instruction", "Student Assessment", "SEN Support", "Parent Communication"},
		Education:     []string{"Bachelor of Education", "Early Childhood Education Degree"},
		Certs:         []string{"First Aid Certificate", "Child Protection Training", "SEN Certificate"},
		Tasks: []string{
			"plan and deliver engaging lessons for mixed-ability groups",
			"support the individual development and well-being of children",
			"assess progress and communicate with parents",
			"maintain a safe and structured classroom environment",
			"collaborate with colleagues on curriculum development",
		},
		RemoteChance: 0.05,
	},
	{
		Name:          "Manufacturing & Industry",
		Companies:     []string{"Vulcan Metalworks", "Apex Precision Manufacturing", "Delta Polymer Industries", "Northgate Assembly", "CoreWorks Industrial", "Briteline Packaging"},
		Titles:        []string{"Machine Operator", "Production Worker", "Quality Inspector", "Maintenance Technician", "Assembly Operator", "Set-Up Technician"},
		ManagerTitles: []string{"Shift Supervisor", "Production Manager"},
		Skills:        []string{"CNC Operation", "Machine Setup", "Quality Control", "Preventive Maintenance", "Assembly Line Work", "Lean Manufacturing", "Technical Drawing", "Workplace Safety"},
		Education:     []string{"Vocational Programme in Mechanical Engineering", "Apprenticeship in Industrial Maintenance"},
		Certs:         []string{"Workplace Safety Certificate", "Forklift License", "First Aid Certificate"},
		Tasks: []string{
			"operate and monitor production machinery",
			"perform in-process quality checks and log results",
			"carry out preventive maintenance and minor repairs",
			"keep the line running to takt time and quality targets",
			"follow safety procedures and 5S standards",
		},
		RemoteChance: 0.02,
	},
	{
		Name:          "Marketing & Creative",
		Companies:     []string{"Pulsewave Agency", "Crane & Co Creative", "Northwind Studio", "Brightside Media", "Lumen Advertising", "Studio Nova"},
		Titles:        []string{"Graphic Designer", "Social Media Manager", "Content Writer", "Marketing Assistant", "Photographer", "Video Editor"},
		ManagerTitles: []string{"Marketing Manager", "Head of Creative"},
		Skills:        []string{"Adobe Photoshop", "Adobe Illustrator", "Copywriting", "Social Media Management", "SEO", "Email Marketing", "Video Editing", "Brand Management", "Content Strategy"},
		Education:     []string{"Bachelor of Marketing", "Bachelor of Design"},
		Certs:         []string{"Google Analytics Certificate", "HubSpot Content Marketing"},
		Tasks: []string{
			"produce visuals and campaign assets across channels",
			"plan and schedule social media content",
			"write copy for web, email and print",
			"track campaign performance and report on KPIs",
			"collaborate with clients and internal teams on briefs",
		},
		RemoteChance: 0.40,
	},
	{
		Name:          "Real Estate & Property",
		Companies:     []string{"Keystone Realty", "Harborline Estates", "Central Park Realtors", "Dwellzone Real Estate", "Anchor Property Management", "Sunrise Estates"},
		Titles:        []string{"Real Estate Agent", "Property Administrator", "Leasing Consultant", "Building Concierge", "Maintenance Coordinator", "Property Assistant"},
		ManagerTitles: []string{"Property Manager", "Head of Leasing"},
		Skills:        []string{"Property Viewings", "Lease Management", "Tenant Relations", "Negotiation", "Maintenance Coordination", "Rent Collection", "Property Listings", "Customer Service"},
		Education:     []string{"Bachelor of Real Estate Management", "High School Diploma"},
		Certs:         []string{"Real Estate License", "First Aid Certificate"},
		Tasks: []string{
			"conduct viewings and guide clients through transactions",
			"prepare listings and market available properties",
			"manage leases, inspections and tenant requests",
			"coordinate repairs with contractors and suppliers",
			"keep property documentation accurate and current",
		},
		RemoteChance: 0.10,
	},
	{
		Name:          "Security & Facilities",
		Companies:     []string{"SentinelGuard Security", "MetroFacility Services", "ClearPath Security Group", "BrightSpace Facility Management", "NightWatch Security", "PrimeCare Facility Services"},
		Titles:        []string{"Security Guard", "Facility Assistant", "Cleaning Specialist", "Porter", "CCTV Operator"},
		ManagerTitles: []string{"Facility Manager", "Head of Security"},
		Skills:        []string{"Access Control", "CCTV Monitoring", "Incident Reporting", "Cleaning Procedures", "Building Maintenance", "Night Rounds", "Customer Service", "Emergency Response"},
		Education:     []string{"High School Diploma", "Vocational Programme in Facility Services"},
		Certs:         []string{"Security Guard License", "First Aid Certificate", "Fire Safety Training"},
		Tasks: []string{
			"monitor premises and control access points",
			"carry out scheduled patrols and incident reporting",
			"keep common areas clean and presentable",
			"handle minor repairs and facility requests",
			"respond calmly to alarms and emergencies",
		},
		RemoteChance: 0.03,
	},
	{
		Name:          "Agriculture & Outdoors",
		Companies:     []string{"Old Mill Farm", "GreenRidge Nurseries", "Valley Harvest Co-op", "Timberline Forestry", "Sunfield Greenhouses", "Meadowbrook Landscaping"},
		Titles:        []string{"Gardener", "Landscaper", "Farm Worker", "Greenhouse Worker", "Forestry Worker", "Groundskeeper"},
		ManagerTitles: []string{"Farm Manager", "Nursery Manager"},
		Skills:        []string{"Lawn Care", "Tree Pruning", "Planting", "Irrigation Systems", "Harvesting", "Landscape Design", "Machinery Operation", "Plant Health"},
		Education:     []string{"Vocational Programme in Horticulture", "High School Diploma"},
		Certs:         []string{"Pesticide Handling Certificate", "Tractor Operation License", "First Aid Certificate"},
		Tasks: []string{
			"maintain gardens, lawns and outdoor spaces",
			"plant, water and care for crops and plants",
			"operate small machinery and hand tools safely",
			"harvest and sort produce to quality standards",
			"support seasonal workloads during peak periods",
		},
		RemoteChance: 0.01,
	},
}

var Cities = []string{
	"Springfield", "Riverton", "Lakeview", "Fairhaven", "Summit City",
	"Mapleton", "Harbor City", "Kingsport", "Ashford", "Burlington",
}

var Closers = []string{
	"Full-time position. Apply with your CV and a short cover note.",
	"This is a full-time, long-term opportunity with an established team.",
	"Immediate start available for the right candidate.",
	"Part-time and full-time options are available.",
}

var Traits = []string{
	"reliability", "attention to detail", "teamwork", "customer focus",
	"safety awareness", "initiative", "punctuality",
}

func GeneratePositions(rng *rand.Rand, n int) []Position {
	perDomain := n / len(Domains)
	remainder := n % len(Domains)

	var positions []Position
	perDomainCounts := make(map[string]int, len(Domains))
	positionIndex := 0
	for domainIndex, domain := range Domains {
		quota := perDomain
		if domainIndex < remainder {
			quota++
		}
		perDomainCounts[domain.Name] = quota
		for k := 0; k < quota; k++ {
			positions = append(positions, NewPosition(rng, domainIndex, domain, positionIndex))
			positionIndex++
		}
	}
	rng.Shuffle(len(positions), func(i, j int) { positions[i], positions[j] = positions[j], positions[i] })

	remoteCount := 0
	managerCount := 0
	for _, p := range positions {
		remoteCount += p.IsRemote
		if strings.Contains(p.Title, "Manager") || strings.Contains(p.Title, "Supervisor") ||
			strings.Contains(p.Title, "Principal") || strings.Contains(p.Title, "Head of") {
			managerCount++
		}
	}
	slog.Debug("position mix", "remote", remoteCount, "managers", managerCount)

	return positions
}

func NewPosition(rng *rand.Rand, domainIndex int, domain Domain, positionIndex int) Position {
	isManager := rng.Intn(100) < 18
	var senior, title string
	if isManager {
		title = Pick(rng, domain.ManagerTitles)
		if rng.Intn(100) < 25 {
			senior = "Senior "
		}
	} else {
		title = Pick(rng, domain.Titles)
		switch roll := rng.Intn(100); {
		case roll < 22:
			senior = "Junior "
		case roll < 55:
			senior = "Senior "
		case roll < 68:
			senior = "Lead "
		}
	}
	fullTitle := senior + title

	skillCount := map[string]int{"Junior ": 3, "": 4, "Senior ": 5, "Lead ": 5}[senior]
	if isManager {
		skillCount = 5
	}
	skills := PickN(rng, domain.Skills, skillCount)
	for len(skills) < 5 {
		skills = append(skills, "")
	}

	remoteChance := domain.RemoteChance
	if isManager {
		remoteChance *= 0.5
	}
	isRemote := 0
	if rng.Float64() < remoteChance {
		isRemote = 1
	}

	items := PickN(rng, domain.Tasks, 3)
	var tasks string
	if len(items) == 0 {
		tasks = ""
	} else if len(items) == 1 {
		tasks = items[0]
	} else {
		var s strings.Builder
		s.WriteString(items[0])
		for _, t := range items[1:] {
			s.WriteString(", ")
			s.WriteString(t)
		}
		tasks = s.String()
	}

	article := Article(fullTitle)

	city := Pick(rng, Cities)

	closer := fmt.Sprintf("On-site role in %s. %s", city, Pick(rng, Closers))
	if isRemote == 1 {
		closer = "Remote-friendly: most of the work can be done from home. " + Pick(rng, Closers)
	}

	description := fmt.Sprintf(
		"%s is looking for %s %s in %s. You will %s. We expect solid experience with %s and %s, a hands-on attitude, and full commitment to safety and quality standards. %s",
		Pick(rng, domain.Companies),
		article,
		fullTitle,
		city,
		tasks,
		skills[0], skills[1],
		closer,
	)

	return Position{
		ID:          positionIndex,
		Title:       fullTitle,
		Description: description,
		IsRemote:    isRemote,
		Skill1:      skills[0],
		Skill2:      skills[1],
		Skill3:      skills[2],
		Skill4:      skills[3],
		Skill5:      skills[4],
		DomainIndex: domainIndex,
	}
}

func Article(title string) string {
	if title == "" {
		return "a"
	}
	if strings.HasPrefix(title, "HVAC") || strings.HasPrefix(title, "HR") {
		return "an"
	}
	switch strings.ToLower(title)[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an"
	default:
		return "a"
	}
}

var AsOf = time.Date(2026, time.October, 31, 12, 0, 0, 0, time.UTC)

func GenerateCandidates(rng *rand.Rand, n int) []Candidate {
	order := rng.Perm(len(Domains))

	candidates := make([]Candidate, 0, n)
	for i := range n {
		primary := order[i%len(Domains)]
		eligible := make([]int, 0, len(Domains)-1)
		for d := range Domains {
			if d != primary {
				eligible = append(eligible, d)
			}
		}
		secondary := Pick(rng, eligible)
		candidate := NewCandidate(rng, primary, secondary, i)
		slog.Debug(
			"candidate primary/secondary domains",
			"index", i,
			"primary", Domains[primary].Name,
			"secondary", Domains[secondary].Name,
		)
		candidates = append(candidates, candidate)
	}
	return candidates
}

func NewCandidate(rng *rand.Rand, primary int, secondary int, candidateIndex int) Candidate {
	primaryDomain, secondaryDomain := Domains[primary], Domains[secondary]

	primarySkills := PickN(rng, primaryDomain.Skills, 5+rng.Intn(3))
	secondarySkills := PickN(rng, secondaryDomain.Skills, 2+rng.Intn(2))

	skillSet := map[string]bool{}
	for _, s := range primarySkills {
		skillSet[s] = true
	}
	for _, s := range secondarySkills {
		skillSet[s] = true
	}

	years := 4 + rng.Intn(15)
	cursor := AsOf.AddDate(-years, -rng.Intn(12), 0)
	now := AsOf.AddDate(0, 0, 1)

	var candidateExperience []CandidateExperience

	if rng.Intn(100) < 40 {
		certificateStart := AsOf.AddDate(-years, 3+rng.Intn(12), 0)
		candidateExperience = append(candidateExperience, CandidateExperience{
			Title:          Pick(rng, primaryDomain.Certs),
			StartedAt:      certificateStart.Format(time.RFC3339),
			EndedAt:        certificateStart.AddDate(0, 1+rng.Intn(6), 0).Format(time.RFC3339),
			ExperienceType: "certification",
		})
	}

	if rng.Intn(100) < 70 {
		start := cursor
		end := start.AddDate(2+rng.Intn(3), 0, 0)
		if end.Before(now) {
			educationTitle := Pick(rng, primaryDomain.Education)
			educationType := "education"
			if rng.Intn(100) < 25 {
				educationType = "apprenticeship"
				educationTitle = Pick(rng, primaryDomain.Titles) + " Apprentice"
			}
			candidateExperience = append(candidateExperience, CandidateExperience{
				Title:          educationTitle,
				StartedAt:      start.Format(time.RFC3339),
				EndedAt:        end.Format(time.RFC3339),
				ExperienceType: educationType,
			})
			cursor = end
		}
	}

	jobCount := 1 + rng.Intn(3)
	for range jobCount {
		end := cursor.AddDate(1+rng.Intn(3), rng.Intn(12), 0)
		if end.After(now) {
			break
		}
		jobSkills := PickN(rng, primaryDomain.Skills, 2+rng.Intn(3))
		for len(jobSkills) < 5 {
			jobSkills = append(jobSkills, "")
		}
		description := ""
		if rng.Intn(100) < 45 {
			description = fmt.Sprintf("Worked in a team of %d, focusing on %s and %s.", 5+rng.Intn(25), jobSkills[0], jobSkills[1])
		}
		candidateExperience = append(candidateExperience, CandidateExperience{
			Title:          SeniorityRoll(rng) + Pick(rng, primaryDomain.Titles),
			StartedAt:      cursor.Format(time.RFC3339),
			EndedAt:        end.Format(time.RFC3339),
			Description:    description,
			Company:        Pick(rng, primaryDomain.Companies),
			ExperienceType: "work",
			Skill1:         jobSkills[0],
			Skill2:         jobSkills[1],
			Skill3:         jobSkills[2],
			Skill4:         jobSkills[3],
			Skill5:         jobSkills[4],
		})
		cursor = end
	}

	workSkills := make([]string, 0, 5)
	seen := map[string]bool{}
	for _, s := range append(append([]string{}, primarySkills[:3]...), secondarySkills...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		workSkills = append(workSkills, s)
	}
	for len(workSkills) < 5 {
		workSkills = append(workSkills, "")
	}
	workSkills = workSkills[:min(5, len(workSkills))]

	workTitle := SeniorityRoll(rng) + Pick(rng, primaryDomain.Titles)

	workCompany := Pick(rng, primaryDomain.Companies)

	candidateExperience = append(candidateExperience, CandidateExperience{
		Title:          workTitle,
		StartedAt:      cursor.Format(time.RFC3339),
		Company:        workCompany,
		ExperienceType: "work",
		Skill1:         workSkills[0],
		Skill2:         workSkills[1],
		Skill3:         workSkills[2],
		Skill4:         workSkills[3],
		Skill5:         workSkills[4],
	})

	sort.Slice(candidateExperience, func(i, j int) bool {
		startedAtI, _ := time.Parse(time.RFC3339, candidateExperience[i].StartedAt)
		startedAtJ, _ := time.Parse(time.RFC3339, candidateExperience[j].StartedAt)
		return startedAtI.After(startedAtJ)
	})

	about := fmt.Sprintf(
		"%d+ years of hands-on experience across %s roles, most recently as %s %s at %s. Practical background in %s, with a secondary skill set in %s. Known for %s and a down-to-earth working style.",
		years,
		primaryDomain.Name,
		Article(workTitle),
		workTitle,
		workCompany,
		primarySkills[0],
		secondarySkills[0],
		Pick(rng, Traits),
	)

	return Candidate{
		About:                about,
		CandidateExperiences: candidateExperience,
		ID:                   candidateIndex,
		PrimaryIndex:         primary,
		SecondaryIndex:       secondary,
		SkillSet:             skillSet,
		MostRecentTitle:      workTitle,
		MostRecentCompany:    workCompany,
	}
}

func SeniorityRoll(rng *rand.Rand) string {
	switch roll := rng.Intn(100); {
	case roll < 25:
		return "Junior "
	case roll < 65:
		return "Senior "
	case roll < 80:
		return "Lead "
	default:
		return ""
	}
}

type PositionScore struct {
	Position Position
	Score    float64
}

func PositionScoreLess(a, b PositionScore) bool {
	return a.Score > b.Score ||
		(a.Score == b.Score && a.Position.ID < b.Position.ID)
}

var SkillBits = func() map[string]uint16 {
	m := make(map[string]uint16)
	for _, d := range Domains {
		for _, s := range d.Skills {
			if _, ok := m[s]; !ok {
				m[s] = uint16(len(m))
			}
		}
	}
	return m
}()

type SkillMask [4]uint64

func SkillMaskOf(skills ...string) SkillMask {
	var m SkillMask
	for _, s := range skills {
		if s == "" {
			continue
		}
		bit, ok := SkillBits[s]
		if !ok || bit >= 256 {
			continue
		}
		m[bit/64] |= 1 << (bit % 64)
	}
	return m
}

func CandidateSkillList(c Candidate) []string {
	skills := make([]string, 0, len(c.SkillSet))
	for s := range c.SkillSet {
		skills = append(skills, s)
	}
	return skills
}

func (m SkillMask) PopCountAnd(other SkillMask) int {
	n := 0
	for i := range m {
		n += bits.OnesCount64(m[i] & other[i])
	}
	return n
}

func PushTop(scores []PositionScore, score PositionScore, limit int) []PositionScore {
	if limit == 0 {
		return scores
	}
	if n := len(scores); n == limit {
		if !PositionScoreLess(score, scores[n-1]) {
			return scores
		}
		scores[n-1] = score
	} else {
		scores = append(scores, score)
	}
	for j := len(scores) - 1; j > 0 && PositionScoreLess(scores[j], scores[j-1]); j-- {
		scores[j], scores[j-1] = scores[j-1], scores[j]
	}
	return scores
}

func GenerateRecommendations(rng *rand.Rand, positions []Position, candidates []Candidate, top int) []Recommendation {
	positionMasks := make([]SkillMask, len(positions))
	for i, p := range positions {
		positionMasks[i] = SkillMaskOf(p.Skill1, p.Skill2, p.Skill3, p.Skill4, p.Skill5)
	}

	needPrimaries := max(top-2, 0)
	needSecondaries := min(2, top)

	recommendations := make([]Recommendation, 0, len(candidates))
	recommendationsIndex := 0

	for i, candidate := range candidates {
		candidateMask := SkillMaskOf(slices.Collect(maps.Keys(candidate.SkillSet))...)

		var primaries, secondaries []PositionScore

		for j, p := range positions {
			matched := candidateMask.PopCountAnd(positionMasks[j])

			var base float64
			switch p.DomainIndex {
			case candidate.PrimaryIndex:
				base = 0.65
			case candidate.SecondaryIndex:
				base = 0.56
			default:
				base = 0.04
			}

			score := base + 0.05*float64(matched) + (rng.Float64()-0.5)*0.06
			capped := PositionScore{Position: p, Score: min(score, 0.98)}

			switch p.DomainIndex {
			case candidate.PrimaryIndex:
				primaries = PushTop(primaries, capped, needPrimaries)
			case candidate.SecondaryIndex:
				secondaries = PushTop(secondaries, capped, needSecondaries)
			}
		}

		pool := make([]PositionScore, 0, needPrimaries+needSecondaries)
		pool = append(pool, primaries...)
		pool = append(pool, secondaries...)

		sort.Slice(pool, func(i, j int) bool {
			return PositionScoreLess(pool[i], pool[j])
		})

		ranked := make([]RankedPosition, 0, top)
		for rank := 0; rank < top && rank < len(pool); rank++ {
			positionScore := pool[rank]
			skills := [5]string{
				positionScore.Position.Skill1,
				positionScore.Position.Skill2,
				positionScore.Position.Skill3,
				positionScore.Position.Skill4,
				positionScore.Position.Skill5,
			}

			var matched []string
			for _, skill := range skills {
				if skill != "" && candidate.SkillSet[skill] {
					matched = append(matched, skill)
				}
			}

			var skill1, skill2 string
			if len(matched) >= 2 {
				skill1, skill2 = matched[0], matched[1]
			} else if len(matched) == 1 {
				skill1 = matched[0]
			}

			domains := Domains[positionScore.Position.DomainIndex]

			var reasoning string
			switch {
			case positionScore.Score >= 0.80 && skill1 != "" && skill2 != "":
				reasoning = fmt.Sprintf(
					"Strong overlap on %s and %s; the candidate's background as %s %s at %s fits the %s role directly.",
					skill1, skill2,
					Article(candidate.MostRecentTitle),
					candidate.MostRecentTitle,
					candidate.MostRecentCompany,
					positionScore.Position.Title,
				)
			case positionScore.Score >= 0.65 && skill1 != "":
				reasoning = fmt.Sprintf(
					"Solid %s experience including %s, matching the day-to-day responsibilities of %s.",
					domains.Name, skill1, positionScore.Position.Title,
				)
			case positionScore.Score >= 0.50:
				reasoning = fmt.Sprintf(
					"Relevant %s background with transferable skills; the candidate is a plausible fit for %s.",
					domains.Name, positionScore.Position.Title,
				)
			default:
				reasoning = fmt.Sprintf(
					"Limited direct overlap, but the candidate's experience as %s %s could transfer to %s.",
					Article(candidate.MostRecentTitle),
					candidate.MostRecentTitle,
					positionScore.Position.Title,
				)
			}

			ranked = append(ranked, RankedPosition{
				Rank:       rank + 1,
				Score:      float64(int(positionScore.Score*100+0.5)) / 100,
				PositionID: positionScore.Position.ID,
				Reasoning:  reasoning,
			})
		}

		recommendations = append(recommendations, Recommendation{
			ID:          recommendationsIndex,
			CandidateID: candidate.ID,
			Positions:   ranked,
		})
		recommendationsIndex++

		if (i+1)%100 == 0 {
			slog.Debug("recommendations progress", "done", i+1, "total", len(candidates))
		}
	}

	return recommendations
}

func Validate(out string, wantPositions, wantCandidates int) error {
	positions := MustReadJSONL[Position](filepath.Join(out, "positions.jsonl"))
	candidates := MustReadJSONL[Candidate](filepath.Join(out, "candidates.jsonl"))
	recommendations := MustReadJSONL[Recommendation](filepath.Join(out, "recommendations.jsonl"))

	if len(positions) != wantPositions {
		return fmt.Errorf("validation failed: positions: got %d, want %d", len(positions), wantPositions)
	}
	if len(candidates) != wantCandidates {
		return fmt.Errorf("validation failed: candidates: got %d, want %d", len(candidates), wantCandidates)
	}

	positionIDs := map[int]bool{}
	for _, p := range positions {
		positionIDs[p.ID] = true
	}
	candidateIDs := map[int]bool{}
	for _, c := range candidates {
		candidateIDs[c.ID] = true
	}
	if len(positionIDs) != len(positions) || len(candidateIDs) != len(candidates) {
		return fmt.Errorf("duplicate ids detected")
	}

	for _, r := range recommendations {
		if !candidateIDs[r.CandidateID] {
			return fmt.Errorf("recommendation references unknown candidate %v", r.CandidateID)
		}
		if len(r.Positions) == 0 {
			return fmt.Errorf("empty recommendation for candidate %v", r.CandidateID)
		}
		for i, rp := range r.Positions {
			if rp.Rank != i+1 {
				return fmt.Errorf("broken rank sequence (%d at index %d) for candidate %v", rp.Rank, i, r.CandidateID)
			}
			if !positionIDs[rp.PositionID] {
				return fmt.Errorf("recommendation references unknown position %v", rp.PositionID)
			}
			if i > 0 && rp.Score > r.Positions[i-1].Score {
				return fmt.Errorf("scores not descending at rank %d for candidate %v", rp.Rank, r.CandidateID)
			}
		}
	}

	if len(recommendations) != len(candidates) {
		return fmt.Errorf("validation failed: recommendations: got %d, want %d", len(recommendations), len(candidates))
	}

	slog.Info(
		"validation passed",
		"positions", len(positions),
		"candidates", len(candidates),
		"recommendations", len(recommendations),
		"max_top", MaxTopOf(recommendations),
	)

	return nil
}

func MaxTopOf(recommendations []Recommendation) int {
	top := 0
	for _, r := range recommendations {
		if len(r.Positions) > top {
			top = len(r.Positions)
		}
	}
	return top
}

func WriteJSON(path string, v any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Error("failed to create output dir", "path", filepath.Dir(path), "err", err)
		panic(err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		panic(err)
	}
}

func MustReadJSONL[T any](path string) []T {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var (
		out []T
		dec = json.NewDecoder(bytes.NewReader(data))
	)
	for {
		var v T
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}
		out = append(out, v)
	}
	return out
}

func Pick[T any](rng *rand.Rand, xs []T) T {
	return xs[rng.Intn(len(xs))]
}

func PickN[T any](rng *rand.Rand, xs []T, n int) []T {
	pool := append([]T{}, xs...)
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if n > len(pool) {
		n = len(pool)
	}
	return pool[:n]
}
