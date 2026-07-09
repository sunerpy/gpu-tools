package nvidiasmi

import "strings"

const (
	fieldIndex       = "index"
	fieldUUID        = "uuid"
	fieldName        = "name"
	fieldMemoryTotal = "memory.total"
	fieldMemoryUsed  = "memory.used"
	fieldTemperature = "temperature.gpu"
	fieldPowerDraw   = "power.draw"
	fieldPowerLimit  = "power.limit"
	fieldClockGR     = "clocks.gr"
	fieldClockMem    = "clocks.mem"
	fieldUtilGPU     = "utilization.gpu"
	fieldUtilMem     = "utilization.memory"
	fieldPState      = "pstate"
	fieldDriver      = "driver_version"
	fieldEncoderUtil = "utilization.encoder"
	fieldDecoderUtil = "utilization.decoder"
	fieldPCIeGen     = "pcie.link.gen.current"
	fieldPCIeWidth   = "pcie.link.width.current"
)

var wantedFields = []string{
	fieldIndex,
	fieldUUID,
	fieldName,
	fieldMemoryTotal,
	fieldMemoryUsed,
	fieldTemperature,
	fieldPowerDraw,
	fieldPowerLimit,
	fieldClockGR,
	fieldClockMem,
	fieldUtilGPU,
	fieldUtilMem,
	fieldPState,
	fieldDriver,
	fieldEncoderUtil,
	fieldDecoderUtil,
	fieldPCIeGen,
	fieldPCIeWidth,
}

var mandatoryFields = map[string]bool{
	fieldIndex: true,
	fieldUUID:  true,
}

func (c *Collector) queryFields() ([]string, error) {
	if c.supportedFields == nil {
		fields, err := supportedFields(c.runner, c.smiPath, "--help-query-gpu")
		if err != nil {
			return nil, err
		}
		c.supportedFields = fields
	}
	fields := make([]string, 0, len(wantedFields))
	for _, field := range wantedFields {
		if mandatoryFields[field] || c.supportedFields[field] {
			fields = append(fields, field)
		}
	}
	return fields, nil
}

func queryArgs(fields []string) []string {
	return []string{
		"--query-gpu=" + strings.Join(fields, ","),
		"--format=csv,noheader,nounits",
	}
}

func fieldIndexes(fields []string) map[string]int {
	indexes := make(map[string]int, len(fields))
	for index, field := range fields {
		indexes[field] = index
	}
	return indexes
}
