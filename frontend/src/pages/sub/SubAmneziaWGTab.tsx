import ConfigBlock from '@/components/clients/ConfigBlock';
import { parseLinkParts } from '@/lib/xray/link-label';
import SubQrButton from './SubQrButton';

// Maximum byte payload of a single QR code at error correction level L.
const MAX_QR_BYTES = 2953;

interface SubAmneziaWGTabProps {
  configs: ReadonlyMap<string, string>;
  onCopy: (value: string) => void;
}

export default function SubAmneziaWGTab({ configs, onCopy }: SubAmneziaWGTabProps) {
  const encoder = new TextEncoder();

  return (
    <div className="sub-rows">
      {Array.from(configs, ([link, config], index) => {
        const fileBase = `awg-${index + 1}`;
        const name = parseLinkParts(link)?.remark || fileBase;

        return (
          <ConfigBlock
            key={link}
            label="AWG"
            name={name}
            text={config}
            fileName={`${fileBase}.conf`}
            qrAction={
              encoder.encode(config).length <= MAX_QR_BYTES && (
                <SubQrButton value={config} label={name} onCopy={onCopy} nativeConfig />
              )
            }
            tagColor="purple"
          />
        );
      })}
    </div>
  );
}
