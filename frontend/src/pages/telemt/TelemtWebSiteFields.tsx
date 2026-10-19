import { useState } from 'react';
import { Alert, Button, Collapse, Input, Select, Upload } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormField } from '@/components/form/rhf';

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
  const [uploadError, setUploadError] = useState('');
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
                  setUploadError('');
                  setValue(
                    prefix,
                    { mode: value, ...(value === 'generate' ? { theme: 'notes' } : {}) },
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
                      value: 'html',
                      label: ru ? 'Загрузить готовый HTML' : 'Upload ready-made HTML',
                    },
                  ]}
                />
              </FormField>
              {mode === 'generate' && (
                <>
                  <FormField name={`${prefix}.theme`} label={ru ? 'Тема страницы' : 'Page theme'}>
                    <Select
                      disabled={disabled}
                      options={[
                        { value: 'notes', label: ru ? 'Личный блог' : 'Personal journal' },
                        { value: 'technology', label: ru ? 'Технологии' : 'Technology' },
                        { value: 'travel', label: ru ? 'Путешествия' : 'Travel' },
                        { value: 'nature', label: ru ? 'Природа' : 'Nature' },
                      ]}
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
              {mode === 'html' && (
                <>
                  <Upload
                    accept=".html,.htm,text/html"
                    maxCount={1}
                    showUploadList={false}
                    disabled={disabled}
                    beforeUpload={async (file) => {
                      try {
                        if (file.size > 256 * 1024)
                          throw new Error(
                            ru ? 'Максимальный размер — 256 КиБ.' : 'Maximum size: 256 KiB.',
                          );
                        const text = await file.text();
                        if (!/<html[\s>]/i.test(text) || text.includes('\0'))
                          throw new Error(
                            ru
                              ? 'Нужен полный HTML-документ.'
                              : 'A complete HTML document is required.',
                          );
                        setValue(`${prefix}.html`, text, {
                          shouldDirty: true,
                          shouldValidate: true,
                        });
                        setUploadError('');
                      } catch (error) {
                        setUploadError(error instanceof Error ? error.message : String(error));
                      }
                      return false;
                    }}
                  >
                    <Button disabled={disabled}>
                      {ru ? 'Выбрать HTML-файл' : 'Choose HTML file'}
                    </Button>
                  </Upload>
                  {uploadError && <Alert type="error" showIcon message={uploadError} />}
                  <FormField
                    name={`${prefix}.html`}
                    label={ru ? 'HTML страницы' : 'Page HTML'}
                    extra={
                      ru
                        ? 'Можно загрузить заранее скачанный шаблон или вставить HTML. До 256 КиБ; CSS и изображения включите в файл. ZIP и папки не поддерживаются. Страница появится после сохранения.'
                        : 'Upload a downloaded template or paste HTML. Up to 256 KiB; embed CSS and images in the file. ZIP archives and folders are not supported. The page goes live after saving.'
                    }
                  >
                    <Input.TextArea disabled={disabled} rows={5} />
                  </FormField>
                </>
              )}
            </>
          ),
        },
      ]}
    />
  );
}
