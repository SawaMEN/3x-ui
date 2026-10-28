import { Alert, Collapse, Input, Select } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormField } from '@/components/form/rhf';
import { TelemtWebCatalogs } from '@/schemas/telemt-web-catalog';

export default function TelemtWebSiteFields({
  prefix = 'site',
  disabled = false,
}: {
  prefix?: string;
  disabled?: boolean;
}) {
  const { control, setValue } = useFormContext();
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru');
  const mode = useWatch({ control, name: `${prefix}.mode` }) as string | undefined;
  const catalog = useWatch({ control, name: `${prefix}.catalog` }) as
    | 'builtin'
    | 'mtproxyl'
    | undefined;
  const template = useWatch({ control, name: `${prefix}.template` }) as string | undefined;
  const selectedCatalog = catalog === 'mtproxyl' ? 'mtproxyl' : 'builtin';
  const templates = TelemtWebCatalogs[selectedCatalog].templates;
  const selectedTemplate = templates.find((entry) => entry.id === template);
  return (
    <Collapse
      items={[
        {
          key: 'site',
          label: ru
            ? 'Сайт-заглушка: настроить по желанию'
            : 'Cover website: optional customization',
          children: (
            <>
              <FormField
                name={`${prefix}.mode`}
                label={ru ? 'Вариант сайта' : 'Website source'}
                transform={{ input: (value) => value || 'auto' }}
                onAfterChange={(value) => {
                  setValue(
                    prefix,
                    {
                      mode: value,
                      ...(value === 'generate' ? { theme: 'notes' } : {}),
                      ...(value === 'catalog' ? { catalog: 'builtin', template: 'notes' } : {}),
                    },
                    { shouldDirty: true, shouldValidate: true },
                  );
                }}
                extra={
                  ru
                    ? 'По умолчанию панель всё делает сама. Выбор сайта не меняет домен, сертификат и ссылки Telegram.'
                    : 'The default is fully automatic. Choosing a site does not change the domain, certificate or Telegram links.'
                }
              >
                <Select
                  disabled={disabled}
                  options={[
                    {
                      value: 'auto',
                      label: ru ? 'Автоматически — простой сайт' : 'Automatic — simple website',
                    },
                    {
                      value: 'generate',
                      label: ru ? 'Создать страницу по теме' : 'Generate a themed page',
                    },
                    {
                      value: 'catalog',
                      label: ru
                        ? 'Выбрать из готовых каталогов'
                        : 'Choose from ready-made catalogs',
                    },
                    {
                      value: 'origin',
                      label: ru ? 'Локальный HTTP-сайт' : 'Local HTTP website',
                    },
                    ...(mode === 'html'
                      ? [
                          {
                            value: 'html',
                            label: ru
                              ? 'Ранее установленная страница'
                              : 'Previously installed page',
                            disabled: true,
                          },
                        ]
                      : []),
                  ]}
                />
              </FormField>
              {mode === 'origin' && (
                <FormField
                  name={`${prefix}.origin`}
                  label={ru ? 'Локальный HTTP-адрес' : 'Local HTTP origin'}
                  extra={
                    ru
                      ? 'Только 127.0.0.1 или ::1 с портом. Обычные запросы к WEB-домену будут направляться этому локальному сайту.'
                      : 'Only 127.0.0.1 or ::1 with a port. Ordinary requests to the WEB domain are forwarded to this local website.'
                  }
                >
                  <Input placeholder="http://127.0.0.1:8080" disabled={disabled} />
                </FormField>
              )}
              {mode === 'generate' && (
                <>
                  <FormField name={`${prefix}.theme`} label={ru ? 'Тема страницы' : 'Page theme'}>
                    <Select
                      disabled={disabled}
                      options={TelemtWebCatalogs.builtin.templates.map((entry) => ({
                        value: entry.id,
                        label: ru ? entry.ru : entry.en,
                      }))}
                    />
                  </FormField>
                  <FormField
                    name={`${prefix}.title`}
                    label={ru ? 'Название (необязательно)' : 'Title (optional)'}
                  >
                    <Input disabled={disabled} maxLength={80} showCount />
                  </FormField>
                  <FormField
                    name={`${prefix}.description`}
                    label={ru ? 'Вступление (необязательно)' : 'Introduction (optional)'}
                    extra={
                      ru
                        ? 'Создаётся локально из встроенных шаблонов: без ИИ-сервисов, ключей и оплаты. Пустые поля заполняются по теме.'
                        : 'Generated locally from bundled templates: no AI service, API key or payment. Empty fields use theme defaults.'
                    }
                  >
                    <Input.TextArea disabled={disabled} maxLength={300} showCount rows={3} />
                  </FormField>
                </>
              )}
              {mode === 'catalog' && (
                <>
                  <FormField
                    name={`${prefix}.catalog`}
                    label={ru ? 'Каталог' : 'Catalog'}
                    onAfterChange={(value) =>
                      setValue(`${prefix}.template`, value === 'mtproxyl' ? 'stub' : 'notes', {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                    }
                  >
                    <Select
                      disabled={disabled}
                      options={Object.entries(TelemtWebCatalogs).map(([value, entry]) => ({
                        value,
                        label: ru ? entry.ru : entry.en,
                      }))}
                    />
                  </FormField>
                  <FormField
                    name={`${prefix}.template`}
                    label={ru ? 'Готовая страница' : 'Ready-made page'}
                  >
                    <Select
                      disabled={disabled}
                      options={templates.map((entry) => ({
                        value: entry.id,
                        label: ru ? entry.ru : entry.en,
                      }))}
                    />
                  </FormField>
                  <Alert
                    type="info"
                    showIcon
                    title={
                      selectedTemplate
                        ? ru
                          ? selectedTemplate.ru
                          : selectedTemplate.en
                        : ru
                          ? 'Выберите страницу'
                          : 'Choose a page'
                    }
                    description={
                      <>
                        <p>
                          {selectedTemplate &&
                            (ru ? selectedTemplate.descriptionRu : selectedTemplate.descriptionEn)}
                        </p>
                        {ru
                          ? 'Нажмите «Сохранить» или «Применить»: панель сама установит страницу и все её ресурсы. Каталоги входят в панель; файлы, ссылки, ключи и дополнительные настройки не нужны. Сайт можно открыть по ссылке выше после сохранения.'
                          : 'Save or apply: the panel installs the page and all its assets automatically. Catalogs are bundled with the panel; no files, URLs, keys or extra settings are needed. Open the website using the link above after saving.'}
                      </>
                    }
                  />
                </>
              )}
              {mode === 'html' && (
                <Alert
                  type="info"
                  showIcon
                  title={
                    ru
                      ? 'Ранее загруженная страница сохранена'
                      : 'Previously uploaded page preserved'
                  }
                  description={
                    ru
                      ? 'Она продолжит работать. Чтобы заменить её, выберите готовый каталог, генерацию по теме или автоматический вариант.'
                      : 'It will keep working. Choose a catalog, themed generation or automatic setup to replace it.'
                  }
                />
              )}
            </>
          ),
        },
      ]}
    />
  );
}
