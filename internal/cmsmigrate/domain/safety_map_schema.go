package domain

// SafetyMapSchema returns the canonical declaration that cmsmigrate applies
// in production. The 19 fields here match proto/v1/safetymap.proto's
// SafetyIncident message 1:1; adding a field in one place requires adding it
// in the other.
func SafetyMapSchema() SchemaDefinition {
	return SchemaDefinition{
		Project: ProjectDefinition{
			Alias:       "overseas-safety-map",
			Name:        "Overseas Safety Map",
			Description: "外務省 海外安全情報を地図で可視化するための CMS プロジェクト",
		},
		Models: []ModelDefinition{safetyIncidentModel()},
	}
}

func safetyIncidentModel() ModelDefinition {
	return ModelDefinition{
		Alias:         "safety-incident",
		Name:          "SafetyIncident",
		Description:   "MOFA open data を 1 件 1 レコードで保持するモデル",
		KeyFieldAlias: "key_cd",
		Fields: []FieldDefinition{
			{Alias: "key_cd", Name: "Key CD", Type: FieldTypeText, Required: true, Unique: true},
			{Alias: "info_type", Name: "Info Type", Type: FieldTypeText, Required: true},
			{Alias: "info_name", Name: "Info Name", Type: FieldTypeText},
			{Alias: "leave_date", Name: "Leave Date", Type: FieldTypeDate, Required: true},
			{Alias: "title", Name: "Title", Type: FieldTypeText, Required: true},
			{Alias: "lead", Name: "Lead", Type: FieldTypeTextArea},
			{Alias: "main_text", Name: "Main Text", Type: FieldTypeTextArea},
			{Alias: "info_url", Name: "Info URL", Type: FieldTypeURL},
			{Alias: "koukan_cd", Name: "Koukan CD", Type: FieldTypeText},
			{Alias: "koukan_name", Name: "Koukan Name", Type: FieldTypeText},
			{Alias: "area_cd", Name: "Area CD", Type: FieldTypeText},
			{Alias: "area_name", Name: "Area Name", Type: FieldTypeText},
			{Alias: "country_cd", Name: "Country CD", Type: FieldTypeText, Required: true},
			{Alias: "country_name", Name: "Country Name", Type: FieldTypeText},
			{Alias: "extracted_location", Name: "Extracted Location", Type: FieldTypeText},
			{Alias: "geometry", Name: "Geometry", Type: FieldTypeGeometryObject},
			{Alias: "geocode_source", Name: "Geocode Source", Type: FieldTypeText},
			{Alias: "ingested_at", Name: "Ingested At", Type: FieldTypeDate, Required: true},
			{Alias: "updated_at", Name: "Updated At", Type: FieldTypeDate, Required: true},
		},
	}
}
