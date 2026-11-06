package service

type TelegramPageVisibility struct{ Local, Remote, Web bool }

func (s *SettingService) GetTelegramPageVisibility() (TelegramPageVisibility, error) {
	var result TelegramPageVisibility
	for key, target := range map[string]*bool{
		"subTelegramLocalEnable":  &result.Local,
		"subTelegramRemoteEnable": &result.Remote,
		"subTelegramWebEnable":    &result.Web,
	} {
		value, err := s.getBool(key)
		if err != nil {
			return result, err
		}
		*target = value
	}
	return result, nil
}
