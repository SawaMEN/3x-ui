import { Alert, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

import { TelemtWebProxyRequestSchema } from '@/schemas/telemt-web';

export default function TelemtWebGuide({ domain }: { domain?: string }) {
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru');
  const parsed = TelemtWebProxyRequestSchema.shape.domain.safeParse(domain);
  return (
    <Alert
      type="info"
      showIcon
      message={ru ? 'Сайт-заглушка — автоматически' : 'Cover website — automatic'}
      description={
        <>
          <p>
            {ru
              ? 'По умолчанию панель создаст простой сайт с заметками. Другую тему или страницу из готовых каталогов можно выбрать в необязательных настройках ниже. В браузере посетитель увидит сайт, а Telegram с WEB-ссылкой подключится к прокси на том же домене. Отдельный хостинг, сайт или порт для заглушки не нужны. Существующий сайт сохраняется при повторной настройке.'
              : 'By default setup creates a simple notes website. Choose another theme or a ready-made catalog page in the optional settings below. Browser visitors see the website; Telegram with a WEB link connects to the proxy on the same domain. No separate hosting, website or public port is needed. Reconfiguration preserves an existing site.'}
          </p>
          <p>
            {ru
              ? 'Клиент Telegram должен поддерживать WEB Proxy. Это отдельная WEB-ссылка, а не обычный MTProto. Для совместимости с iOS оставьте основной транспорт HTTPS; перебор позволяет совместимым клиентам выбрать WebSocket.'
              : 'The Telegram client must support WEB Proxy and use a WEB link. Keep HTTPS as the primary transport for iOS compatibility; negotiation lets compatible clients choose WebSocket.'}
          </p>
          {parsed.success && (
            <Typography.Link
              href={`https://${parsed.data}/`}
              target="_blank"
              rel="noopener noreferrer"
            >
              {ru ? 'Открыть сайт (после сохранения)' : 'Open website (after saving)'}
            </Typography.Link>
          )}
        </>
      }
    />
  );
}

export const telemtWebFieldHelp = {
  ru: {
    frontend:
      'Программа, которая принимает защищённые подключения на порту 443, использует TLS-сертификат и передаёт трафик локальному Telemt. Если не уверены, оставьте nginx: панель установит и настроит его сама.',
    carrier:
      'Способ передачи Telegram-трафика внутри HTTPS (WEB carrier). HTTPS — совместимый основной вариант, в том числе для iOS. WebSocket — постоянное соединение; варианты lanes разделяют потоки по отдельным каналам.',
    carriers:
      'Выберите варианты в порядке попыток: сначала выбранный первым. Пустой список отключает автоматический выбор. Клиенты без поддержки выбора используют основной транспорт. Уже работающие соединения не переключаются.',
  },
  en: {
    frontend:
      'The HTTPS frontend accepts encrypted connections on port 443, uses the TLS certificate and forwards traffic to local Telemt. Leave nginx selected if unsure: the panel installs and configures it automatically.',
    carrier:
      'How Telegram traffic travels inside HTTPS (WEB carrier). HTTPS is the compatible primary option, including iOS. WebSocket keeps a persistent connection; lanes variants give streams independent channels.',
    carriers:
      'Select transports in attempt order, first selected first. An empty list disables negotiation. Clients without negotiation use the primary transport. Active connections do not switch transports.',
  },
};
