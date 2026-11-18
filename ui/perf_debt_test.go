package ui

// Performance debt: fixtures over a hard budget today, with the value
// measured and why. TestPerfDamage / TestPerfBudgets fail on a new
// overrun and on an entry that is within budget again (delete it then).

type perfDebt struct {
	Measured string // e.g. "3.1ms" or "dirty 4.2×"
	Why      string
}

// perfDamageDebt is keyed by fixture name. Every entry (all families) was
// regenerated from one verbose TestPerfDamage run (task-5-fix-2-damage.log)
// with the content-area denominator (perfContentRects / perfContentArea:
// union area of the shown parts, transparent overlay carriers excluded);
// task-6-damage.log measured every earlier entry again unchanged and set
// the collection entries. task-M1-damage.log (after the origin/main
// merge) re-measured every entry: changed values were updated (same
// full-window dirty area, smaller/larger content bbox upstream) and the
// post-merge fixtures were added.
var perfDamageDebt = map[string]perfDebt{
	// Task 3 display/feedback: measured after reversible interactions pass.
	"Alert":          {Measured: "dirty 9.90×", Why: "baseline"},
	"Avatar":         {Measured: "dirty 810.00×", Why: "baseline"},
	"AvatarGroup":    {Measured: "dirty 316.41×", Why: "baseline"},
	"Badge":          {Measured: "dirty 810.00×", Why: "baseline"},
	"BadgeRibbon":    {Measured: "dirty 7.39×", Why: "baseline"},
	"Calendar":       {Measured: "dirty 14.06×", Why: "baseline"},
	"Card":           {Measured: "dirty 21.18×", Why: "baseline"},
	"Carousel":       {Measured: "dirty 4.66×", Why: "baseline"},
	"CheckableTag":   {Measured: "dirty 1042.64×", Why: "baseline"},
	"CodeBlock":      {Measured: "dirty 5.50×", Why: "baseline"},
	"Collapse":       {Measured: "dirty 10.55×", Why: "baseline"},
	"Countdown":      {Measured: "dirty 16.05×", Why: "baseline"},
	"Descriptions":   {Measured: "dirty 6.13×", Why: "baseline"},
	"Empty":          {Measured: "dirty 5.23×", Why: "baseline"},
	"Image":          {Measured: "dirty 33.75×", Why: "baseline"},
	"JSONView":       {Measured: "dirty 5.33×", Why: "baseline"},
	"JSONViewSource": {Measured: "dirty 5.33×", Why: "baseline"},
	"JSONViewValue":  {Measured: "dirty 5.33×", Why: "baseline"},
	"Loading":        {Measured: "dirty 7.76×", Why: "baseline"},
	"Markdown":       {Measured: "dirty 9.00×", Why: "baseline"},
	"Popconfirm":     {Measured: "dirty 55.59×", Why: "baseline"},
	"Popover":        {Measured: "dirty 64.22×", Why: "baseline"},
	"Progress":       {Measured: "dirty 19.53×", Why: "baseline"},
	"QRCode":         {Measured: "dirty 50.62×", Why: "baseline"},
	"Rate":           {Measured: "dirty 366.75×", Why: "baseline"},
	"Result":         {Measured: "dirty 6.53×", Why: "baseline"},
	"RichText":       {Measured: "dirty 89.01×", Why: "baseline"},
	"Segmented":      {Measured: "dirty 208.36×", Why: "baseline"},
	"Skeleton":       {Measured: "dirty 16.07×", Why: "baseline"},
	"SkeletonAvatar": {Measured: "dirty 810.00×", Why: "baseline"},
	"SkeletonButton": {Measured: "dirty 632.81×", Why: "baseline"},
	"SkeletonImage":  {Measured: "dirty 105.47×", Why: "baseline"},
	"SkeletonInput":  {Measured: "dirty 162.00×", Why: "baseline"},
	"Spin":           {Measured: "dirty 179.00×", Why: "baseline"},
	"Statistic":      {Measured: "dirty 16.05×", Why: "baseline"},
	"StatisticTimer": {Measured: "dirty 16.05×", Why: "baseline"},
	"Tag":            {Measured: "dirty 467.47×", Why: "baseline"},
	"Timeline":       {Measured: "dirty 3.88×", Why: "baseline"},
	"Tooltip":        {Measured: "dirty 143.33×", Why: "baseline"},

	"App":              {Measured: "dirty 18.00×", Why: "baseline"},
	"AspectRatio":      {Measured: "dirty 14.40×", Why: "baseline"},
	"BorderBeam":       {Measured: "dirty 33.75×", Why: "baseline"},
	"ButtonGroup":      {Measured: "dirty 210.08×", Why: "baseline"},
	"CollapsibleSider": {Measured: "dirty 5.33×", Why: "baseline"},
	"Compact":          {Measured: "dirty 198.54×", Why: "baseline"},
	"ConfigProvider":   {Measured: "dirty 284.91×", Why: "baseline"},
	"Custom":           {Measured: "dirty 14.55×", Why: "baseline"},
	"Disabled":         {Measured: "dirty 391.36×", Why: "baseline"},
	"Div":              {Measured: "dirty 40.50×", Why: "baseline"},
	"Divider":          {Measured: "dirty 38.79×", Why: "baseline"},
	"Flex":             {Measured: "dirty 84.38×", Why: "baseline"},
	"GridRow":          {Measured: "dirty 42.32×", Why: "baseline"},
	"Icon":             {Measured: "dirty 2250.00×", Why: "baseline"},
	"IconButton":       {Measured: "dirty 2677.69×", Why: "baseline"},
	"Kbd":              {Measured: "dirty 1592.02×", Why: "baseline"},
	"Layout":           {Measured: "dirty 4.74×", Why: "baseline"},
	"Link":             {Measured: "dirty 42.32×", Why: "baseline"},
	"Paragraph":        {Measured: "dirty 40.91×", Why: "baseline"},
	"Row":              {Measured: "dirty 36.16×", Why: "baseline"},
	"Scroll":           {Measured: "dirty 6.33×", Why: "baseline"},
	"ScrollH":          {Measured: "dirty 16.88×", Why: "baseline"},
	"Space":            {Measured: "dirty 42.32×", Why: "baseline"},
	"Splitter":         {Measured: "dirty 8.91×", Why: "baseline"},
	"StackBox":         {Measured: "dirty 33.75×", Why: "baseline"},
	"Text":             {Measured: "dirty 42.32×", Why: "baseline"},
	"Title":            {Measured: "dirty 24.50×", Why: "baseline"},
	"Watermark":        {Measured: "dirty 6.33×", Why: "baseline"},

	"Button":                   {Measured: "dirty 454.10×", Why: "baseline"},
	"Input":                    {Measured: "dirty 112.50×", Why: "baseline"},
	"Select":                   {Measured: "dirty 27.00×", Why: "baseline"},
	"Addon":                    {Measured: "dirty 27.72×", Why: "baseline"},
	"AutoComplete":             {Measured: "dirty 26.47×", Why: "baseline"},
	"Checkbox":                 {Measured: "dirty 725.59×", Why: "baseline"},
	"CheckAll":                 {Measured: "dirty 17.90×", Why: "baseline"},
	"CheckboxGroup":            {Measured: "dirty 200.98×", Why: "baseline"},
	"ColorPicker":              {Measured: "dirty 18.35×", Why: "baseline"},
	"ColorPickerGradient":      {Measured: "dirty 13.88×", Why: "baseline"},
	"ColorPickerPanel":         {Measured: "dirty 15.13×", Why: "baseline"},
	"ColorPickerPanelGradient": {Measured: "dirty 12.56×", Why: "baseline"},
	"DropZone":                 {Measured: "dirty 20.77×", Why: "baseline"},
	"FieldSet":                 {Measured: "dirty 29.09×", Why: "baseline"},
	"DatePicker":               {Measured: "dirty 12.23×", Why: "baseline"},
	"DatePickerPanel":          {Measured: "dirty 14.61×", Why: "baseline"},
	"DateTimePicker":           {Measured: "dirty 7.85×", Why: "baseline"},
	"MultiDatePicker":          {Measured: "dirty 13.36×", Why: "baseline"},
	"TimePanel":                {Measured: "dirty 33.83×", Why: "baseline"},
	"TimeRangePicker":          {Measured: "dirty 10.65×", Why: "baseline"},
	"TimeSelect":               {Measured: "dirty 28.12×", Why: "baseline"},
	"Form":                     {Measured: "dirty 29.09×", Why: "baseline"},
	"HotkeyInput":              {Measured: "dirty 155.77×", Why: "baseline"},
	"InputNumber":              {Measured: "dirty 184.09×", Why: "baseline"},
	"Item":                     {Measured: "dirty 11.64×", Why: "baseline"},
	"ItemGroup":                {Measured: "dirty 5.82×", Why: "baseline"},
	"RadioGroup":               {Measured: "dirty 200.98×", Why: "baseline"},
	"SwitchValue":              {Measured: "dirty 545.38×", Why: "baseline"},
	"InputTag":                 {Measured: "dirty 112.50×", Why: "baseline"},
	"MarkdownEditor":           {Measured: "dirty 4.63×", Why: "baseline"},
	"Mentions":                 {Measured: "dirty 112.50×", Why: "baseline"},
	"OTP":                      {Measured: "dirty 174.57×", Why: "baseline"},
	"Radio":                    {Measured: "dirty 362.80×", Why: "baseline"},
	"RangePicker":              {Measured: "dirty 6.98×", Why: "baseline"},
	"Search":                   {Measured: "dirty 155.77×", Why: "baseline"},
	"Slider":                   {Measured: "dirty 21.16×", Why: "baseline"},
	"SliderRange":              {Measured: "dirty 42.32×", Why: "baseline"},
	"Switch":                   {Measured: "dirty 502.69×", Why: "baseline"},
	"TextArea":                 {Measured: "dirty 30.86×", Why: "baseline"},
	"TimePicker":               {Measured: "dirty 26.01×", Why: "baseline"},
	"Toggle":                   {Measured: "dirty 908.20×", Why: "baseline"},
	"ToggleGroup":              {Measured: "dirty 218.31×", Why: "baseline"},
	"Upload":                   {Measured: "dirty 10.35×", Why: "baseline"},

	// Task 5 navigation/overlays.
	"Affix":              {Measured: "dirty 5.62×", Why: "baseline"},
	"Anchor":             {Measured: "dirty 2.59×", Why: "baseline"},
	"BackTop":            {Measured: "dirty 5.59×", Why: "baseline"},
	"Breadcrumb":         {Measured: "dirty 21.16×", Why: "baseline"},
	"BreadcrumbItems":    {Measured: "dirty 32.53×", Why: "baseline"},
	"Command":            {Measured: "dirty 11.14×", Why: "baseline"},
	"ContextMenuAt":      {Measured: "dirty 5.39×", Why: "baseline"},
	"ContextMenuAtWith":  {Measured: "dirty 18.75×", Why: "baseline"},
	"Dropdown":           {Measured: "dirty 49.44×", Why: "baseline"},
	"FloatButton":        {Measured: "dirty 810.00×", Why: "baseline"},
	"FloatButtonGroup":   {Measured: "dirty 213.16×", Why: "baseline"},
	"Menu":               {Measured: "dirty 18.24×", Why: "baseline"},
	"Menubar":            {Measured: "dirty 18.40×", Why: "baseline"},
	"Message":            {Measured: "dirty 26.30×", Why: "baseline"},
	"Notification":       {Measured: "dirty 17.40×", Why: "baseline"},
	"NotificationCenter": {Measured: "dirty 11.68×", Why: "baseline"},
	"Overlay":            {Measured: "dirty 64.06×", Why: "baseline"},
	"Pagination":         {Measured: "dirty 29.09×", Why: "baseline"},
	"Steps":              {Measured: "dirty 17.24×", Why: "baseline"},
	"Tabs":               {Measured: "dirty 20.24×", Why: "baseline"},

	// Task 6 collections.
	"Cascader":       {Measured: "dirty 168.75×", Why: "baseline"},
	"CascaderPanel":  {Measured: "dirty 21.62×", Why: "baseline"},
	"InfiniteScroll": {Measured: "dirty 10.38×", Why: "baseline"},
	"List":           {Measured: "dirty 3.75×", Why: "baseline"},
	"ListActions":    {Measured: "dirty 4.31×", Why: "baseline"},
	"Listy":          {Measured: "dirty 2.59×", Why: "baseline"},
	"LogView":        {Measured: "dirty 4.22×", Why: "baseline"},
	"MultiSelect":    {Measured: "dirty 17.46×", Why: "baseline"},
	"Transfer":       {Measured: "dirty 16.20×", Why: "baseline"},
	"Tree":           {Measured: "dirty 4.16×", Why: "baseline"},
	"TreeSelect":     {Measured: "dirty 168.75×", Why: "baseline"},
	"VirtualList":    {Measured: "dirty 8.44×", Why: "baseline"},
	"VirtualSelect":  {Measured: "dirty 168.75×", Why: "baseline"},
	"VirtualTree":    {Measured: "dirty 2.91×", Why: "baseline"},
	"ThoughtChain":   {Measured: "dirty 7.63×", Why: "baseline"},
	"Sender":         {Measured: "dirty 11.35×", Why: "baseline"},
	// Task M1 post-merge fixtures (task-M1-damage.log).
	"CellEditorTable":   {Measured: "dirty 2.82×", Why: "baseline"},
	"CheckableTagGroup": {Measured: "dirty 316.72×", Why: "baseline"},
	"ListToolBar":       {Measured: "dirty 9.90×", Why: "baseline"},
	"ProBreadcrumb":     {Measured: "dirty 21.16×", Why: "baseline"},
	"RowEditorTable":    {Measured: "dirty 2.82×", Why: "baseline"},
	"TableDropdown":     {Measured: "dirty 93.94×", Why: "baseline"},
	"TopNavHeader":      {Measured: "dirty 16.63×", Why: "baseline"},
}

// perfTimeDebt is keyed by "Name/Scene". LogView's scenes are owned by
// perf/memory-logview: an overrun of one gets an entry with Why "owned by
// perf/memory-logview" instead of a fix on this branch (none is over its
// budget: the Task R1 baseline measured Mount, Interact and Large100k
// within).
//
// Task R1 baseline (merged tree 1969e04, adaptive perfTime, Apple M4,
// 1-min load < 6): the worse of 2 TestPerfBudgets runs (-p 1). A
// Large100k budget is min(4ms, 1.5 × Large1k) of the same run.
//
// Batch 3 V2: List, ProList and Conversations Large100k are NA (default
// non-virtual form, O(n) by design; perfNANonVirtual) and left this table;
// their LargeVirtual scenes are budgeted instead.
var perfTimeDebt = map[string]perfDebt{
	"AvatarGroup/Large100k":     {Measured: "26.50ms (budget 237µs)", Why: "baseline"},
	"BubbleList/Large100k":      {Measured: "6.77ms (budget 562µs)", Why: "baseline"},
	"Carousel/Large100k":        {Measured: "360.46ms (budget 2.24ms)", Why: "baseline"},
	"CellEditorTable/Large100k": {Measured: "508.92ms (budget 4.00ms)", Why: "baseline"},
	"CheckAll/Large100k":        {Measured: "2.65s (budget 2.22ms)", Why: "baseline"},
	"CheckboxGroup/Large100k":   {Measured: "2.66s (budget 2.22ms)", Why: "baseline"},
	"Collapse/Large100k":        {Measured: "1.91s (budget 4.00ms)", Why: "baseline"},
	"Descriptions/Large100k":    {Measured: "2.22s (budget 4.00ms)", Why: "baseline"},
	"Image/Mount":               {Measured: "689µs (budget 500µs)", Why: "baseline"},
	"InfiniteScroll/Large100k":  {Measured: "1.02s (budget 3.92ms)", Why: "baseline"},
	"InputTag/Large100k":        {Measured: "2.36s (budget 4.00ms)", Why: "baseline"},
	"Markdown/Large100k":        {Measured: "1.23s (budget 4.00ms)", Why: "baseline"},
	"Masonry/Large100k":         {Measured: "478µs (budget 97µs)", Why: "baseline"},
	"Menu/Large100k":            {Measured: "135.50ms (budget 826µs)", Why: "baseline"},
	"ProTable/Large100k":        {Measured: "361.55ms (budget 3.09ms)", Why: "baseline"},
	"RadioGroup/Large100k":      {Measured: "2.63s (budget 2.35ms)", Why: "baseline"},
	"RichText/Large100k":        {Measured: "2.18s (budget 4.00ms)", Why: "baseline"},
	"RowEditorTable/Large100k":  {Measured: "500.34ms (budget 4.00ms)", Why: "baseline"},
	"Scroll/Large100k":          {Measured: "62.59ms (budget 494µs)", Why: "baseline"},
	"ScrollH/Large100k":         {Measured: "63.11ms (budget 501µs)", Why: "baseline"},
	"Segmented/Large100k":       {Measured: "1.18s (budget 1.38ms)", Why: "baseline"},
	"Tabs/Large100k":            {Measured: "4.73s (budget 3.88ms)", Why: "baseline"},
	"Timeline/Large100k":        {Measured: "1.55s (budget 4.00ms)", Why: "baseline"},
	"ToggleGroup/Large100k":     {Measured: "2.62s (budget 1.42ms)", Why: "baseline"},
	"Transfer/Large100k":        {Measured: "86.12ms (budget 940µs)", Why: "baseline"},
	"Tree/Large100k":            {Measured: "1.28s (budget 4.00ms)", Why: "baseline"},
	"TreeSelect/Large100k":      {Measured: "7.75ms (budget 65µs)", Why: "baseline"},
}
