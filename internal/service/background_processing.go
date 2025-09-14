package service

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/version"
	"github.com/rs/zerolog"
)

type BackgroundProcesseror interface {
	ProcessService(ctx context.Context, service *model.Service) error
}

type BackgroundProcessorConfig struct {
	Versioner *version.Versioner
}

func NewBackgroundProcessor(config BackgroundProcessorConfig) (BackgroundProcesseror, error) {
	if config.Versioner == nil {
		return nil, fmt.Errorf("versioner not provided")
	}
	return &backgroundProcessor{versioner: *config.Versioner}, nil
}

type backgroundProcessor struct {
	versioner version.Versioner
}

func (bp *backgroundProcessor) ProcessService(ctx context.Context, service *model.Service) error {
	log := zerolog.Ctx(ctx)
	nextVersion, err := bp.versioner.CalculateNextVersion(ctx, service.GitRepoFilePath)
	if err != nil {
		return err
	}
	log.Info().Interface("semver", nextVersion).Str("nextVersion", nextVersion.String()).Msg("Next version")
	return nil
}
