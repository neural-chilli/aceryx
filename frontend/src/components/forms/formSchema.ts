export interface FormSchema {
  title?: string
  layout?: Section[]
  actions?: Action[]
  fields?: FieldDef[]
}

export interface Section {
  section: string
  fields: FieldDef[]
}

export interface FieldDef {
  id?: string
  bind: string
  label?: string
  type?: string
  readonly?: boolean
  required?: boolean
  options?: string[]
  options_from?: string
  min_length?: number
  max_length?: number
  min?: number
  max?: number
}

export interface Action {
  label: string
  value: string
  style?: string
  requires?: string[]
}
