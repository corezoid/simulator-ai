package mcpserver

// smartFormTools are the console's Smart Form references on public methods.
var smartFormTools = []parityTool{
	{Name: "smart_form_components", Method: "cdu/catalog/get", BriefUnlessNarrowed: true, Description: "The CDU component catalog a Smart Form page config is validated against. Without types or kind it lists every type with its kind, category and label (brief); then call it with types for the ones you need: their props (the JSON Schema the validator applies), events, state and api. Use ONLY these types and props: sim-api refuses an unknown type or prop when the page is saved, naming the closest ones. A field caption is the label prop of TextInput or Select; DatePicker has none, so put a Label before it."},
}
