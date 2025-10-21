package restful

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/api_response"
	"github.com/okieraised/monitoring-agent/internal/cerrors"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/utilities"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"go.opentelemetry.io/otel/trace"
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
	Host    HostInfo    `json:"host"`
	Memory  MemoryInfo  `json:"memory"`
	Network NetworkInfo `json:"network"`
	CPU     CPUInfo     `json:"cpu"`
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

func (svc *HealthcheckService) Healthcheck(ctx *gin.Context, input *HealthcheckInput) (*api_response.BaseOutput, *cerrors.AppError) {

	hostStat, err := host.Info()
	if err != nil {
		return nil, nil
	}
	memoryInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, nil
	}
	physicalMacs, err := utilities.RetrievePhysicalMacAddr()
	if err != nil {
		return nil, nil
	}
	outboundIP, err := utilities.GetOutboundIP()
	if err != nil {
		return nil, nil
	}
	cpuStat, err := cpu.Info()
	if err != nil {
		return nil, nil
	}
	physicalCores, err := cpu.Counts(false)
	if err != nil {
		return nil, nil
	}

	logicalCores, err := cpu.Counts(true)
	if err != nil {
		return nil, nil
	}

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
	}

	return &api_response.BaseOutput{
		Code:    cerrors.OK.Code,
		Message: cerrors.OK.Message,
		Data:    respData,
	}, nil
}
