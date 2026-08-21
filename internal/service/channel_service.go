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

// Name 返回渠道 ID 对应的名称（用于审计详情可读性；不存在返回错误）。
func (s *ChannelService) Name(id int64) (string, error) {
	ch, err := s.repo.GetByID(id)
	if err != nil {
		return "", err
	}
	return ch.Name, nil
}

// List 返回全部未删除渠道（所有用户共享的数据集）；userID 参数仅为兼容保留，不再过滤。
func (s *ChannelService) List(userID int64) ([]*model.Channel, error) {
	list, err := s.repo.List()
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
	if _, ok := channel.Get(in.Type); !ok {
		return errors.New("不支持的渠道类型")
	}
	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return err
	}
	in.ID = id
	// 保持原属主：管理员可编辑任意用户的渠道
	in.UserID = existing.UserID
	in.ConfigJSON = enc
	return s.repo.Update(in)
}

func (s *ChannelService) Delete(userID, id int64) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

// BatchDelete 批量软删除渠道（单条 UPDATE）。
func (s *ChannelService) BatchDelete(ids []int64) error {
	return s.repo.BatchDelete(ids)
}

func (s *ChannelService) Test(userID int64, id int64, cfg map[string]string) error {
	if id > 0 {
		c, err := s.repo.GetByID(id)
		if err != nil {
			return err
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
	// 内置类型在上面的 switch 中总会用解密后的 cfg 构造全新实例。
	// 此回退返回注册表中的共享原型实例，丢弃已解密的 cfg —— 仅适用于忽略
	// config 的测试/插件渠道类型（如 fakeChan）；真实发送依赖 config 的
	// 渠道必须走上面的 switch 分支。暂不重构该接口。
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
