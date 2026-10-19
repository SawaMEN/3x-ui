export const TelemtWebCatalogs = {
  builtin: {
    ru: 'Страницы 3x-ui',
    en: '3x-ui pages',
    templates: [
      {
        id: 'notes',
        ru: 'Личный блог',
        en: 'Personal journal',
        descriptionRu: 'Светлая страница с короткими заметками о повседневной жизни.',
        descriptionEn: 'A light journal with short notes about everyday life.',
      },
      {
        id: 'technology',
        ru: 'Технологии',
        en: 'Technology',
        descriptionRu: 'Заметки о программном обеспечении, инструментах и цифровых привычках.',
        descriptionEn: 'Notes about software, useful tools and digital habits.',
      },
      {
        id: 'travel',
        ru: 'Путешествия',
        en: 'Travel',
        descriptionRu: 'Небольшой журнал о поездках, прогулках и новых местах.',
        descriptionEn: 'A small journal about journeys, walks and new places.',
      },
      {
        id: 'nature',
        ru: 'Природа',
        en: 'Nature',
        descriptionRu: 'Страница о временах года, зелёных пространствах и наблюдениях за природой.',
        descriptionEn: 'A page about seasons, green spaces and nature observations.',
      },
    ],
  },
  mtproxyl: {
    ru: 'Коллекция MTProxyL',
    en: 'MTProxyL collection',
    templates: [
      {
        id: 'stub',
        ru: 'Простая заглушка',
        en: 'Simple placeholder',
        descriptionRu: 'Минимальная страница «Ведутся технические работы», без игры.',
        descriptionEn: 'A minimal maintenance page without a game.',
      },
      {
        id: 'catrunner',
        ru: 'Cat Runner — игра с котом',
        en: 'Cat Runner — cat game',
        descriptionRu: 'Готовая браузерная игра: кот бежит и преодолевает препятствия.',
        descriptionEn: 'A ready-made browser game: a cat runs and avoids obstacles.',
      },
      {
        id: 'mekorunner',
        ru: 'Meko Runner — игра-бегалка',
        en: 'Meko Runner — runner game',
        descriptionRu: 'Ещё один вариант браузерной игры с собственным оформлением.',
        descriptionEn: 'Another browser runner game with its own visual theme.',
      },
    ],
  },
} as const;

export function isTelemtWebCatalogTemplate(catalog?: string, template?: string) {
  if (catalog !== 'builtin' && catalog !== 'mtproxyl') return false;
  return TelemtWebCatalogs[catalog].templates.some((entry) => entry.id === template);
}
