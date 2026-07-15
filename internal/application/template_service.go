package application

type DocumentTemplate struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
}

type TemplateService struct{}

func NewTemplateService() *TemplateService {
	return &TemplateService{}
}

func (s *TemplateService) ListTemplates() []DocumentTemplate {
	return []DocumentTemplate{
		{
			ID:          "sop",
			Title:       "Стандартная операционная процедура (SOP)",
			Description: "Шаблон для регламентации регулярных рабочих процессов и процессов обслуживания.",
			Category:    "Регламенты",
			Tags:        []string{"sop", "процесс", "инструкция"},
			Content: `# [Название процедуры]

> **Область применения:** [Департамент / Отдел]  
> **Ответственный роли:** [Роль / Должность]  

## 1. Цель
Опишите краткую цель выполнения регламента.

## 2. Предварительные требования
- [ ] Требование 1
- [ ] Требование 2

## 3. Пошаговый алгоритм
1. Шаг 1: [Описание]
2. Шаг 2: [Описание]

## 4. Контроль качества и риски
- Риск A: меры предотвращения
`,
		},
		{
			ID:          "incident-report",
			Title:       "Отчет по инциденту (Post-mortem)",
			Description: "Разбор сбоев, хронологии, причин и превентивных мер после инцидентов.",
			Category:    "Эксплуатация",
			Tags:        []string{"incident", "postmortem", "ops"},
			Content: `# Post-Mortem: [Название инцидента]

**Дата происшествия:** YYYY-MM-DD  
**Критичность:** P1 / P2  

## 1. Резюме
Краткое описание того, что произошло и каков был доступность сервиса.

## 2. Влияние на пользователей
- Затронуто пользователей: X%
- Время простоя (Downtime): YY минут

## 3. Хронология событий (UTC)
- **10:00** — Обнаружен всплеск ошибок
- **10:15** — Локализация проблемы
- **10:30** — Применение хотфикса

## 4. Первопричина (Root Cause)
Подробное техническое описание причин инцидента.

## 5. Корректирующие действия
- [ ] Внедрить дополнительные метрики
- [ ] Обновить правила алертинга
`,
		},
		{
			ID:          "meeting-notes",
			Title:       "Протокол встречи (Meeting Notes)",
			Description: "Структура для записи решений, задач и участников совещаний.",
			Category:    "Управление",
			Tags:        []string{"meeting", "protocol"},
			Content: `# Протокол встречи: [Тема]

**Дата:** YYYY-MM-DD  
**Участники:**  

## Обсужденные вопросы
1. Пункт 1
2. Пункт 2

## Принятые решения
- Решение A

## Action Items (Задачи)
- [ ] @пользователь: Выполнить задачу X до YYYY-MM-DD
`,
		},
	}
}

func (s *TemplateService) GetTemplateByID(id string) *DocumentTemplate {
	for _, t := range s.ListTemplates() {
		if t.ID == id {
			return &t
		}
	}
	return nil
}
