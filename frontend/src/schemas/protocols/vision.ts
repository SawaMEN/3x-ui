import { z } from 'zod';

/** Padding lengths are unsigned 16-bit values in the Vision wire format. */
export const VisionTestseedSchema = z
  .array(z.number().int().min(0))
  .refine(
    (seed) =>
      seed.length === 0 ||
      (seed.length === 4 &&
        seed[0] > 0 &&
        seed[1] > 0 &&
        seed[3] > 0 &&
        seed[2] >= seed[0] - 1 &&
        seed[1] + seed[2] - 1 <= 65535 &&
        seed[3] <= 65536),
    'Invalid Vision padding parameters',
  );
