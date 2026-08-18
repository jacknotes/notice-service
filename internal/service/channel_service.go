package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/model"
	"notice-service/internal/repository"
)

type ChannelService struct {
	repo   *repository.ChannelRepo
	cipher *crypto.Cipher
}

func NewChannelService(db *sql.DB, cipher *crypto.Cipher) *ChannelService {
	return &ChannelService{repo: repository.NewChannelRepo(db), cipher: cipher}
}

func (s *ChannelService) List(userID int64) ([]*model.Channel, error) {
	list, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		cfg, err := s.decryptConfig(c.ConfigJSON)
		if err != nil {
			log.Printf("channel %d: decrypt config failed: %v", c.ID, err)
			continue
		}
		c.Config = cfg
	}
	return list, nil
}

func (s *ChannelService) Create(userID int64, in *model.Channel) error {
	if _, ok := channel.Get(in.Type); !ok {
		return errors.New("不支持的渠道类型")
	}
	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return err
	}
	in.UserID = userID
	in.ConfigJSON = enc
	return s.repo.Create(in)
}

func (s *ChannelService) Update(userID, id int64, in *model.Channel) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return errors.New("无权操作")
	}
	if _, ok := channel.Get(in.Type); !ok {
		return errors.New("不支持的渠道类型")
	}
	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return err
	}
	in.ID = id
	in.UserID = userID
	in.ConfigJSON = enc
	return s.repo.Update(in)
}

func (s *ChannelService) Delete(userID, id int64) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return errors.New("无权操作")
	}
	return s.repo.Delete(id)
}

func (s *ChannelService) Test(userID int64, id int64, cfg map[string]string) error {
	if id > 0 {
		c, err := s.repo.GetByID(id)
		if err != nil {
			return err
		}
		if c.UserID != userID {
			return errors.New("无权操作")
		}
		dec, err := s.decryptConfig(c.ConfigJSON)
		if err != nil {
			return err
		}
		cfg = dec
		ch, ok := channel.Get(c.Type)
		if !ok {
			return errors.New("不支持的渠道类型")
		}
		return ch.TestConnection(cfg)
	}
	// 新建前测试：需要 type 从配置传入（handler 会在测试请求里带 type）
	ch, ok := channel.Get(cfg["type"])
	if !ok {
		return errors.New("不支持的渠道类型")
	}
	return ch.TestConnection(cfg)
}

func (s *ChannelService) InstancedChannel(c *model.Channel) (channel.Channel, error) {
	cfg, err := s.decryptConfig(c.ConfigJSON)
	if err != nil {
		return nil, err
	}
	switch c.Type {
	case "email":
		return channel.NewEmailChannel(cfg), nil
	case "wecom":
		return channel.NewWecomChannel(cfg), nil
	case "dingtalk":
		return channel.NewDingtalkChannel(cfg), nil
	case "feishu":
		return channel.NewFeishuChannel(cfg), nil
	case "wechat":
		return channel.NewWechatChannel(cfg), nil
	}
	// 注册表中已注册的类型（含测试/插件渠道）可直接复用。
	if ch, ok := channel.Get(c.Type); ok {
		return ch, nil
	}
	return nil, errors.New("不支持的渠道类型")
}

func (s *ChannelService) encryptConfig(cfg map[string]string) (string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return s.cipher.EncryptString(string(b))
}

func (s *ChannelService) decryptConfig(enc string) (map[string]string, error) {
	if enc == "" {
		return map[string]string{}, nil
	}
	plain, err := s.cipher.DecryptString(enc)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(plain), &m); err != nil {
		return nil, err
	}
	return m, nil
}
