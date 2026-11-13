import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CloseCircleFilled } from '@ant-design/icons';
import { DatePicker, theme } from 'antd';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import { PersianDateTimePicker } from 'persian-calendar-suite';

import { useDatepicker } from '@/hooks/useDatepicker';
import { useTheme } from '@/hooks/useTheme';
import './DateTimePicker.css';

interface DateTimePickerProps {
  value: Dayjs | null;
  onChange: (next: Dayjs | null) => void;
  showTime?: boolean;
  format?: string;
  placeholder?: string;
  disabled?: boolean;
  allowClear?: boolean;
  maxDate?: Dayjs;
  id?: string;
  'aria-label'?: string;
  'aria-describedby'?: string;
}

export default function DateTimePicker({
  value,
  onChange,
  showTime = true,
  format = 'YYYY-MM-DD HH:mm:ss',
  placeholder = '',
  disabled = false,
  allowClear = true,
  maxDate,
  id,
  'aria-label': ariaLabel,
  'aria-describedby': ariaDescribedBy,
}: DateTimePickerProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const jalaliRef = useRef<HTMLDivElement>(null);
  // Bumped on clear: persian-calendar-suite reads `value` only on mount, so
  // remounting via key is the only way to reflect an externally cleared value.
  const [clearNonce, setClearNonce] = useState(0);
  // Mounted without a value, persian-calendar-suite seeds today and emits it —
  // which would instantly undo a clear. Armed across every (re)mount.
  const suppressMountEmit = useRef(true);

  useEffect(() => {
    suppressMountEmit.current = false;
    return () => {
      suppressMountEmit.current = true;
    };
  }, [clearNonce]);

  const persianTheme = useMemo(() => {
    const token = theme.getDesignToken(antdThemeConfig);
    return {
      primaryColor: token.colorPrimary,
      backgroundColor: token.colorBgElevated,
      borderColor: token.colorBorder,
      hoverColor: token.colorPrimaryBg,
      selectedTextColor: 'var(--xui-on-primary)',
      textColor: token.colorText,
    };
  }, [antdThemeConfig]);

  const commitChange = (next: Dayjs | null) => {
    if (next && maxDate && next.isAfter(maxDate)) {
      if (datepicker === 'jalalian') setClearNonce((n) => n + 1);
      return;
    }
    onChange(next);
  };

  // The library hardcodes a Persian placeholder and exposes no working prop to
  // override it, so clear it (or apply the caller's) on the input directly so
  // the empty field shows no leftover Persian text. No dep array: re-apply
  // after every render (incl. clear-remounts).
  useEffect(() => {
    if (datepicker !== 'jalalian') return;
    const input = jalaliRef.current?.querySelector('input');
    if (!input) return;
    input.placeholder = placeholder;
    if (id) input.id = id;
    if (ariaLabel) input.setAttribute('aria-label', ariaLabel);
    if (ariaDescribedBy) input.setAttribute('aria-describedby', ariaDescribedBy);
  });

  if (datepicker === 'jalalian') {
    return (
      <div
        ref={jalaliRef}
        className={`jdp-wrap${isDark ? ' jdp-dark' : ''}${isUltra ? ' jdp-ultra' : ''}${disabled ? ' jdp-disabled' : ''}${value ? '' : ' jdp-empty'}`}
      >
        <PersianDateTimePicker
          key={clearNonce}
          value={value ? value.valueOf() : null}
          onChange={(next: number | string | null) => {
            if (suppressMountEmit.current) return;
            if (next == null || next === '') {
              commitChange(null);
              return;
            }
            const ms = typeof next === 'number' ? next : Number(next);
            if (Number.isFinite(ms)) commitChange(dayjs(ms));
          }}
          showTime={showTime}
          outputFormat="timestamp"
          maxDate={maxDate?.toDate()}
          persianNumbers
          rtlCalendar
          theme={persianTheme}
        />
        {value && allowClear && !disabled && (
          <button
            type="button"
            className="jdp-clear"
            aria-label={t('clear')}
            onMouseDown={(e) => e.preventDefault()}
            onClick={(e) => {
              e.stopPropagation();
              commitChange(null);
              setClearNonce((n) => n + 1);
            }}
          >
            <CloseCircleFilled aria-hidden />
          </button>
        )}
      </div>
    );
  }

  return (
    <DatePicker
      value={value}
      onChange={(next) => commitChange(next || null)}
      onCalendarChange={(next) => commitChange((Array.isArray(next) ? next[0] : next) || null)}
      showTime={showTime ? { format: 'HH:mm:ss' } : false}
      needConfirm={false}
      format={format}
      placeholder={placeholder}
      disabled={disabled}
      allowClear={allowClear}
      maxDate={maxDate}
      id={id}
      aria-label={ariaLabel}
      aria-describedby={ariaDescribedBy}
      style={{ width: '100%' }}
    />
  );
}
