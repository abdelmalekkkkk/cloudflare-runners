package state

import (
	"context"
	"encoding/json"
	"time"

	"github.com/abdelmalekkkkk/cf-runners/cloudflare"
)

const path = "state.json"

type GithubApp struct {
	ID   int
	Name string
	Slug string
	URL  string
}

type State struct {
	GithubApp     *GithubApp
	WorkerID      string
	SecretStoreID string
	WorkerVersion string
	DockerImage   string
	ContainerID   string
	UpdatedAt     time.Time
}

type StateManager struct {
	ctx    context.Context
	client *cloudflare.Client
	state  *State
}

func CreateStateManager(ctx context.Context, client *cloudflare.Client) *StateManager {
	return &StateManager{
		ctx:    ctx,
		client: client,
	}
}

func (s *StateManager) load() error {
	if s.state != nil {
		return nil
	}

	data, err := s.client.GetObject(path)

	if err != nil {
		return err
	}

	if data == nil {
		s.state = &State{}
		return nil
	}

	return json.Unmarshal(data, &s.state)
}

func (s *StateManager) save() error {
	s.state.UpdatedAt = time.Now()

	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}

	err = s.client.PutObject(path, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateManager) SetWorkerID(workerID string) error {
	err := s.load()
	if err != nil {
		return err
	}

	s.state.WorkerID = workerID
	return s.save()
}

func (s *StateManager) GetWorkerID() (string, error) {
	err := s.load()
	if err != nil {
		return "", err
	}

	if s.state == nil {
		return "", nil
	}

	return s.state.WorkerID, nil
}

func (s *StateManager) SetSecretStoreID(secretStoreID string) error {
	err := s.load()
	if err != nil {
		return err
	}

	s.state.SecretStoreID = secretStoreID
	return s.save()
}

func (s *StateManager) GetSecretStoreID() (string, error) {
	err := s.load()
	if err != nil {
		return "", err
	}

	if s.state == nil {
		return "", nil
	}

	return s.state.SecretStoreID, nil
}

func (s *StateManager) SetGithubApp(app GithubApp) error {
	err := s.load()
	if err != nil {
		return err
	}

	s.state.GithubApp = &app
	return s.save()
}

func (s *StateManager) GetGithubApp() (*GithubApp, error) {
	err := s.load()
	if err != nil {
		return nil, err
	}

	if s.state == nil {
		return nil, nil
	}

	return s.state.GithubApp, nil
}

func (s *StateManager) SetWorkerVersion(workerVersion string) error {
	err := s.load()
	if err != nil {
		return err
	}

	s.state.WorkerVersion = workerVersion
	return s.save()
}

func (s *StateManager) GetWorkerVersion() (string, error) {
	err := s.load()
	if err != nil {
		return "", err
	}

	if s.state == nil {
		return "", nil
	}

	return s.state.WorkerVersion, nil
}

func (s *StateManager) SetDockerImage(dockerImage string) error {
	err := s.load()
	if err != nil {
		return err
	}

	s.state.DockerImage = dockerImage
	return s.save()
}

func (s *StateManager) GetDockerImage() (string, error) {
	err := s.load()
	if err != nil {
		return "", err
	}

	if s.state == nil {
		return "", nil
	}

	return s.state.DockerImage, nil
}

func (s *StateManager) SetContainerID(containerID string) error {
	err := s.load()
	if err != nil {
		return err
	}

	s.state.ContainerID = containerID
	return s.save()
}

func (s *StateManager) GetContainerID() (string, error) {
	err := s.load()
	if err != nil {
		return "", err
	}

	if s.state == nil {
		return "", nil
	}

	return s.state.ContainerID, nil
}
