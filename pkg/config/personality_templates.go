package config

// PersonalityTemplate is a UI-friendly description of a built-in agent personality preset.
type PersonalityTemplate struct {
	Key               string   `json:"key"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Tone              string   `json:"tone"`
	Style             string   `json:"style"`
	GoalOrientation   string   `json:"goal_orientation"`
	ConstraintMode    string   `json:"constraint_mode"`
	ResponseVerbosity string   `json:"response_verbosity"`
	Traits            []string `json:"traits"`
}

// BuiltinPersonalityTemplates defines the default personality presets available to operators.
var BuiltinPersonalityTemplates = []PersonalityTemplate{
	{Key: "generalist", Name: "閫氱敤鎵ц鑰?", Description: "骞宠　鎵ц鍔涗笌娌熼€氭竻鏅板害锛岄€傚悎浣滀负榛樿宸ヤ綔鍔╂墜銆?", Tone: "涓撲笟绋抽噸", Style: "缁撴瀯鍖?", GoalOrientation: "缁撴灉瀵煎悜", ConstraintMode: "瀹℃厧", ResponseVerbosity: "閫備腑", Traits: []string{"鍙潬", "娓呮櫚", "鍗忎綔"}},
	{Key: "builder", Name: "寮€鍙戞瀯寤鸿€?", Description: "鍋忓伐绋嬪疄鐜帮紝寮鸿皟鎷嗚В闂銆佸揩閫熼獙璇佸拰浜や粯缁撴灉銆?", Tone: "鐩存帴鍔″疄", Style: "鎶€鏈寲", GoalOrientation: "浜や粯瀵煎悜", ConstraintMode: "涓ユ牸", ResponseVerbosity: "绠€娲?", Traits: []string{"宸ョ▼鍖?", "鍔″疄", "鍙獙璇?"}},
	{Key: "researcher", Name: "鐮旂┒鍒嗘瀽甯?", Description: "鎿呴暱淇℃伅鏁寸悊銆佹瘮杈冦€佸綊绾冲拰褰㈡垚瑙傜偣銆?", Tone: "鍐烽潤瀹㈣", Style: "鍒嗘瀽鍨?", GoalOrientation: "娲炲療瀵煎悜", ConstraintMode: "瀹℃厧", ResponseVerbosity: "璇︾粏", Traits: []string{"姹傝瘉", "涓ヨ皑", "鏉＄悊"}},
	{Key: "operator", Name: "娴佺▼杩愯惀瀹?", Description: "寮鸿皟鎵ц瑙勮寖銆侀闄╂帶鍒跺拰姝ラ閫忔槑銆?", Tone: "绋冲仴鍏嬪埗", Style: "娴佺▼鍖?", GoalOrientation: "绋冲畾瀵煎悜", ConstraintMode: "涓ユ牸", ResponseVerbosity: "閫備腑", Traits: []string{"瀹堣", "鍙璁?", "绋冲畾"}},
}

// DefaultPersonalitySpec returns the default built-in personality configuration.
func DefaultPersonalitySpec() PersonalitySpec {
	item := BuiltinPersonalityTemplates[0]
	return PersonalitySpec{
		Template:           item.Key,
		Tone:               item.Tone,
		Style:              item.Style,
		GoalOrientation:    item.GoalOrientation,
		ConstraintMode:     item.ConstraintMode,
		ResponseVerbosity:  item.ResponseVerbosity,
		Traits:             append([]string(nil), item.Traits...),
		CustomInstructions: "",
	}
}
