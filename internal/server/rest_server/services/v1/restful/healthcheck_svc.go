package restful

import (
	"context"

	"github.com/distatus/battery"
	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/utilities"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type IHealthcheckService interface {
	Healthcheck(ctx *gin.Context, input *HealthcheckInput) (*api_response.BaseOutput, *cerrors.AppError)
}

type HealthcheckService struct {
	logger *log.Logger
}

func NewHealthcheckService(options ...func(*HealthcheckService)) *HealthcheckService {
	svc := &HealthcheckService{}
	for _, opt := range options {
		opt(svc)
	}
	logger := log.MustNewECSLogger()
	svc.logger = logger
	return svc
}

type HealthcheckInput struct {
	TracerCtx context.Context
	Tracer    trace.Tracer
}

type HealthcheckOutput struct {
	Host    HostInfo      `json:"host"`
	Memory  MemoryInfo    `json:"memory"`
	Network NetworkInfo   `json:"network"`
	CPU     CPUInfo       `json:"cpu"`
	Battery []BatteryInfo `json:"batteries"`
}

type MemoryInfo struct {
	Total       uint64  `json:"total"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type NetworkInfo struct {
	OutboundIP   string   `json:"outbound_ip"`
	PhysicalMacs []string `json:"physical_macs"`
}

type HostInfo struct {
	Hostname             string `json:"hostname"`
	OS                   string `json:"os"`
	Platform             string `json:"platform"`
	PlatformFamily       string `json:"platform_family"`
	PlatformVersion      string `json:"platform_version"`
	KernelVersion        string `json:"kernel_version"`
	Arch                 string `json:"arch"`
	VirtualizationSystem string `json:"virtualization_system"`
	VirtualizationRole   string `json:"virtualization_role"`
	HostID               string `json:"host_id"`
}

type CPUInfo struct {
	ModelName     string `json:"model_name"`
	VendorID      string `json:"vendor_id"`
	PhysicalCores int    `json:"physical_cores"`
	LogicalCores  int    `json:"logical_cores"`
}

type BatteryInfo struct {
	State         string  `json:"state"`
	Current       float64 `json:"current"`
	Full          float64 `json:"full"`
	Design        float64 `json:"design"`
	ChargeRate    float64 `json:"charge_rate"`
	Voltage       float64 `json:"voltage"`
	DesignVoltage float64 `json:"design_voltage"`
}

func (svc *HealthcheckService) Healthcheck(ctx *gin.Context, input *HealthcheckInput) (*api_response.BaseOutput, *cerrors.AppError) {
	rootCtx, span := input.Tracer.Start(input.TracerCtx, "healthcheck-handler")
	defer span.End()

	resp := &api_response.BaseOutput{}
	lg := svc.logger.With(
		zap.String(constants.APIFieldRequestID, ctx.GetString(constants.APIFieldRequestID)),
	)

	_, cSpan := input.Tracer.Start(rootCtx, "get-host-info")
	hostStat, err := host.Info()
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get memory info")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	_, cSpan = input.Tracer.Start(rootCtx, "get-memory-info")
	memoryInfo, err := mem.VirtualMemory()
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get memory info")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	_, cSpan = input.Tracer.Start(rootCtx, "get-net-info")
	physicalMacs, err := utilities.RetrievePhysicalMacAddr()
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get physical mac addresses")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}

	outboundIP, err := utilities.GetOutboundIP()
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get outbound ip")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	_, cSpan = input.Tracer.Start(rootCtx, "get-cpu-info")
	cpuStat, err := cpu.Info()
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get cpu info")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}

	physicalCores, err := cpu.Counts(false)
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get cpu info")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}

	logicalCores, err := cpu.Counts(true)
	if err != nil {
		cSpan.End()
		wErr := errors.Wrap(err, "failed to get cpu info")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	cSpan.End()

	_, cSpan = input.Tracer.Start(rootCtx, "get-battery-info")
	batteries, err := battery.GetAll()
	if err != nil {
		wErr := errors.Wrap(err, "failed to get battery info")
		lg.Error(wErr.Error())
		return nil, cerrors.ErrGenericInternalServer
	}
	batteryInfo := make([]BatteryInfo, 0, len(batteries))
	for _, bt := range batteries {
		batteryInfo = append(batteryInfo, BatteryInfo{
			State:         bt.State.String(),
			Current:       bt.Current,
			Full:          bt.Full,
			Design:        bt.Design,
			ChargeRate:    bt.ChargeRate,
			Voltage:       bt.Voltage,
			DesignVoltage: bt.DesignVoltage,
		})
	}
	cSpan.End()

	respData := HealthcheckOutput{
		Host: HostInfo{
			Hostname:             hostStat.Hostname,
			OS:                   hostStat.OS,
			Platform:             hostStat.Platform,
			PlatformFamily:       hostStat.PlatformFamily,
			PlatformVersion:      hostStat.PlatformVersion,
			KernelVersion:        hostStat.KernelVersion,
			Arch:                 hostStat.KernelArch,
			VirtualizationSystem: hostStat.VirtualizationSystem,
			VirtualizationRole:   hostStat.VirtualizationRole,
			HostID:               hostStat.HostID,
		},
		Memory: MemoryInfo{
			Total:       memoryInfo.Total,
			Free:        memoryInfo.Free,
			UsedPercent: memoryInfo.UsedPercent,
		},
		Network: NetworkInfo{
			OutboundIP:   outboundIP.String(),
			PhysicalMacs: physicalMacs,
		},
		CPU: CPUInfo{
			ModelName:     cpuStat[0].ModelName,
			VendorID:      cpuStat[0].VendorID,
			PhysicalCores: physicalCores,
			LogicalCores:  logicalCores,
		},
		Battery: batteryInfo,
	}

	resp.Code = cerrors.OK.Code
	resp.Message = cerrors.OK.Message
	resp.Data = respData

	return resp, nil
}
