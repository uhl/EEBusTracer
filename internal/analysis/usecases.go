package analysis

import (
	"bytes"
	"encoding/json"

	"github.com/eebustracer/eebustracer/internal/model"
)

// UseCaseInfo describes a single detected use case.
type UseCaseInfo struct {
	UseCaseName    string `json:"useCaseName"`
	Abbreviation   string `json:"abbreviation"`
	UseCaseVersion string `json:"useCaseVersion,omitempty"`
	Available      bool   `json:"available"`
	Scenarios      []uint `json:"scenarios,omitempty"`
}

// DeviceUseCases groups detected use cases for a device/actor pair.
type DeviceUseCases struct {
	DeviceAddr string        `json:"deviceAddr"`
	Actor      string        `json:"actor"`
	UseCases   []UseCaseInfo `json:"useCases"`
	MessageID  int64         `json:"messageId"`
}

// useCaseAbbreviations maps SPINE use case names to short abbreviations.
var useCaseAbbreviations = map[string]string{
	"limitationOfPowerConsumption":                          "LPC",
	"limitationOfPowerProduction":                           "LPP",
	"monitoringOfPowerConsumption":                          "MPC",
	"monitoringOfGridConnectionPoint":                       "MGCP",
	"evseCommissioningAndConfiguration":                     "EVSECC",
	"evChargingSummary":                                     "EVCS",
	"evCommissioningAndConfiguration":                       "EVCC",
	"measurementOfElectricityDuringEvCharging":              "MOEEC",
	"optimizationOfSelfConsumptionDuringEvCharging":         "OSCEV",
	"overloadProtectionByEvChargingCurrentCurtailment":      "OPEV",
	"coordinatedEvCharging":                                 "CEC",
	"configurationOfDhwSystemFunction":                      "CDSF",
	"configurationOfDhwTemperature":                         "CDT",
	"configurationOfRoomCoolingSystemFunction":              "CRCSF",
	"configurationOfRoomCoolingTemperature":                 "CRCT",
	"configurationOfRoomHeatingSystemFunction":              "CRHSF",
	"configurationOfRoomHeatingTemperature":                 "CRHT",
	"monitoringOfBattery":                                   "MOB",
	"monitoringOfDhw":                                       "MODHW",
	"monitoringOfDhwSystemFunction":                         "MODSF",
	"monitoringOfDhwTemperature":                            "MODT",
	"monitoringOfInverter":                                  "MOI",
	"monitoringOfPvString":                                  "MOPVS",
	"monitoringOfRoomCoolingSystemFunction":                 "MORCSF",
	"monitoringOfRoomCoolingTemperature":                    "MORCT",
	"monitoringOfRoomHeatingSystemFunction":                 "MORHSF",
	"monitoringOfRoomHeatingTemperature":                    "MORHT",
	"visualizationOfAggregatedBatteryData":                  "VABD",
	"visualizationOfAggregatedPhotovoltaicData":             "VAPD",
	"flexibleStartOfEnergyConsumer":                         "FSEC",
	"flexibleStartOfEnergyProduction":                       "FSEP",
	"monitoringOfPowerConsumptionOfDhwSystemFunction":       "MOPCDSF",
	"monitoringOfPowerConsumptionOfRoomCoolingSystemFunction": "MOPCRCSF",
	"monitoringOfPowerConsumptionOfRoomHeatingSystemFunction": "MOPCRHSF",
}

// UseCaseFunctionSets maps use case abbreviations to the SPINE function sets
// typically associated with that use case. This is a best-effort static mapping.
var UseCaseFunctionSets = map[string][]string{
	"LPC": {"LoadControlLimitListData", "LoadControlLimitDescriptionListData",
		"LoadControlLimitConstraintsListData"},
	"LPP": {"LoadControlLimitListData", "LoadControlLimitDescriptionListData",
		"LoadControlLimitConstraintsListData"},
	"MPC": {"MeasurementListData", "MeasurementDescriptionListData",
		"MeasurementConstraintsListData"},
	"MGCP": {"MeasurementListData", "MeasurementDescriptionListData",
		"MeasurementConstraintsListData"},
	"EVCC": {"DeviceConfigurationKeyValueListData",
		"DeviceConfigurationKeyValueDescriptionListData",
		"IdentificationListData"},
	"MOB":  {"MeasurementListData", "MeasurementDescriptionListData"},
	"MOI":  {"MeasurementListData", "MeasurementDescriptionListData"},
	"MOPVS": {"MeasurementListData", "MeasurementDescriptionListData"},
}

// DetectUseCases parses nodeManagementUseCaseData from messages to identify
// active use cases per device.
func DetectUseCases(msgs []*model.Message) []DeviceUseCases {
	type deviceActorKey struct {
		device, actor string
	}

	resultMap := map[deviceActorKey]*DeviceUseCases{}
	resultOrder := []deviceActorKey{}

	for _, msg := range msgs {
		if msg.FunctionSet != "NodeManagementUseCaseData" {
			continue
		}
		if msg.CmdClassifier != "reply" && msg.CmdClassifier != "notify" {
			continue
		}
		if len(msg.SpinePayload) == 0 {
			continue
		}

		useCaseInfos := extractUseCaseData(msg.SpinePayload)
		if len(useCaseInfos) == 0 {
			continue
		}

		deviceAddr := msg.DeviceSource
		if deviceAddr == "" {
			deviceAddr = msg.SourceAddr
		}

		for _, uci := range useCaseInfos {
			key := deviceActorKey{device: deviceAddr, actor: uci.actor}
			if _, ok := resultMap[key]; !ok {
				resultMap[key] = &DeviceUseCases{
					DeviceAddr: deviceAddr,
					Actor:      uci.actor,
					MessageID:  msg.ID,
				}
				resultOrder = append(resultOrder, key)
			}
			duc := resultMap[key]
			duc.UseCases = uci.useCases
			duc.MessageID = msg.ID
		}
	}

	results := make([]DeviceUseCases, 0, len(resultOrder))
	for _, key := range resultOrder {
		results = append(results, *resultMap[key])
	}
	return results
}

type useCaseInfoGroup struct {
	actor    string
	useCases []UseCaseInfo
}

func extractUseCaseData(spinePayload json.RawMessage) []useCaseInfoGroup {
	cmds := extractCmds(spinePayload)

	var groups []useCaseInfoGroup
	for _, cmd := range cmds {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		raw, ok := cmdMap["nodeManagementUseCaseData"]
		if !ok {
			continue
		}

		// Parse useCaseInformation — handle both array and single-object forms
		ucInfoList := parseJSONArrayOrSingle[ucInfoRaw](raw, "useCaseInformation")

		for _, info := range ucInfoList {
			actor := ""
			if info.Actor != nil {
				actor = *info.Actor
			}

			// Parse useCaseSupport — handle both array and single-object forms
			var useCases []UseCaseInfo
			for _, uc := range info.UseCaseSupport {
				if uc.UseCaseName == nil {
					continue
				}
				name := *uc.UseCaseName
				abbr := useCaseAbbreviations[name]
				if abbr == "" {
					abbr = name
				}

				available := true
				if uc.UseCaseAvailable != nil {
					available = *uc.UseCaseAvailable
				}

				version := ""
				if uc.UseCaseVersion != nil {
					version = *uc.UseCaseVersion
				}

				scenarios := uc.ScenarioSupport

				useCases = append(useCases, UseCaseInfo{
					UseCaseName:    name,
					Abbreviation:   abbr,
					UseCaseVersion: version,
					Available:      available,
					Scenarios:      scenarios,
				})
			}

			if len(useCases) > 0 {
				groups = append(groups, useCaseInfoGroup{
					actor:    actor,
					useCases: useCases,
				})
			}
		}
	}
	return groups
}

// ucInfoRaw is the raw JSON structure for use case information entries.
type ucInfoRaw struct {
	Address *struct {
		Device *string `json:"device"`
	} `json:"address"`
	Actor          *string          `json:"actor"`
	UseCaseSupport []ucSupportRaw   `json:"useCaseSupport"`
}

type ucSupportRaw struct {
	UseCaseName      *string `json:"useCaseName"`
	UseCaseVersion   *string `json:"useCaseVersion"`
	UseCaseAvailable *bool   `json:"useCaseAvailable"`
	ScenarioSupport  []uint  `json:"scenarioSupport"`
}

// parseJSONArrayOrSingle extracts a named field from a JSON object,
// handling both array and single-object forms (EEBUS normalization may
// flatten single-element arrays to plain objects).
func parseJSONArrayOrSingle[T any](data json.RawMessage, field string) []T {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	raw, ok := obj[field]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '[' {
		var items []T
		if err := json.Unmarshal(raw, &items); err == nil {
			return items
		}
	}
	if trimmed[0] == '{' {
		var item T
		if err := json.Unmarshal(raw, &item); err == nil {
			return []T{item}
		}
	}
	return nil
}
