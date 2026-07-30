# Timesheet Table Component

Component type: `timesheet-table` | Default width: `col-12`

A time and expense tracking table for capturing billable items with support for contract services, working hours, and custom entries. Includes tax calculations and AI-powered data import.

## Basic Usage

```json
{
  "component": "timesheet-table",
  "label": "Time & Expenses",
  "properties": {
    "width": "col-12",
    "currencyCode": "USD",
    "timesheetShowTax": false
  }
}
```

## Service Types

The timesheet supports three entry modes:

| Type | Use for |
|---|---|
| `ContractService` | Service-based billing from predefined service options |
| `WorkingHours` | Hourly/calendar-based entries with daily hour tracking |
| `Custom` | Free-form entries not from service options |

## Table Columns

| Column | Always Shown | Description |
|---|---|---|
| Name | Yes | Service/item name (clickable, opens edit modal) |
| Type | Yes | Service type label |
| Rate | Yes | Unit price |
| Quantity | Yes | Total units (auto-calculated for WorkingHours) |
| Tax Rate | If `timesheetShowTax=true` | Tax percentage |
| Total Tax | If `timesheetShowTax=true` | Calculated tax amount |
| Total | Yes | Total cost |
| Actions | If not read-only | Duplicate and delete buttons |

## Data Structure (TimesheetRecord)

Each row stores:
```
{
  id: string,
  name: string,
  type: string,
  rate: number,
  quantity: number,
  total: number,
  taxRate?: number,              // if tax enabled
  totalWithTax?: number,         // if tax enabled
  timesheetServiceType: "ContractService" | "WorkingHours" | "Custom",
  workingHoursDetails?: {        // only for WorkingHours type
    periodStart: string,
    periodEnd: string,
    hours: number[]              // daily hours array
  }
}
```

## Calculation Logic

- **Total**: `quantity x rate + (rate x quantity x taxRate / 100)` if tax enabled
- **Working Hours quantity**: sum of all daily hours in `workingHoursDetails.hours[]`
- **Tax**: `rate x quantity x (taxRate / 100)`

## Layout Properties

### Basic

| Property | Type | Description |
|---|---|---|
| `currencyCode` | string | ISO currency code (default: `"USD"`) |
| `decimals` | number | Display decimal precision |
| `decimalsToInputAndSave` | number | Storage decimal precision |
| `helpText` | string | Help text displayed below component |

### Timesheet-Specific

| Property | Type | Description |
|---|---|---|
| `timesheetShowTax` | boolean | Show tax rate and total tax columns |
| `timesheetCreateButtonLabel` | string | Custom label for "Create" button |
| `timesheetTotalDataPath` | string | Where to save the grand total (auto: `{dataPath}_Total`) |
| `timesheetServiceOptions` | array | Static service option definitions |
| `timesheetServiceOptionsDataPath` | string | Data path to load dynamic service options |
| `timesheetServiceOptionsDataPathMapping` | object | Maps data fields to service option fields |
| `timesheetAddServiceOptions` | string[] | Which service type buttons to show in Create menu |
| `timesheetAddServiceOptionsLabels` | object | Custom labels for service type buttons |
| `timesheetServiceOptionsFilter` | object | Filter rules for service options |
| `minRange` | number | Minimum hours per day for WorkingHours (default: 1) |
| `maxRange` | number | Maximum hours per day for WorkingHours (default: 31) |

### Service Options

Static service options:
```json
{
  "timesheetServiceOptions": [
    {"name": "Consulting", "type": "Professional", "rate": 150},
    {"name": "Development", "type": "Technical", "rate": 200, "taxRate": 10}
  ]
}
```

Dynamic service options from data:
```json
{
  "timesheetServiceOptionsDataPath": "$.services",
  "timesheetServiceOptionsDataPathMapping": {
    "name": "serviceName",
    "type": "serviceCategory",
    "rate": "unitPrice",
    "taxRate": "tax"
  }
}
```

### Service Options Filtering

Filter format: `"field:function(value)"`

Supported functions: `includes`, `notIncludes`, `startsWith`, `endsWith`, `equals`, `notEquals`

**Simple field filter** — filters on the mapped service option fields (name, type, rate):
```json
{
  "timesheetServiceOptionsFilter": {
    "type": "includes(Consulting)",
    "name": "notIncludes(Deprecated)"
  }
}
```

**Per-service-type filter with `record.<field>`** — filters on the original source record's raw fields, scoped by service type. Use this to show different service options for different entry types:
```json
{
  "timesheetServiceOptionsFilter": {
    "ContractService": "record.serviceType:equals(Routine Services)",
    "WorkingHours": "record.serviceType:equals(Variable Services)"
  }
}
```

This is useful when the same service options data source contains items for multiple service types and you want to partition them — e.g., fixed-price items for ContractService and hourly items for WorkingHours.

### Controlling the Create Menu

Limit which service types appear in the "Create" button dropdown:
```json
{
  "timesheetAddServiceOptions": ["ContractService", "WorkingHours"],
  "timesheetAddServiceOptionsLabels": {
    "ContractService": "Add Service Item",
    "WorkingHours": "Log Hours"
  }
}
```

## AI Import

Enable AI-powered data import from files (PDF, CSV, Excel, images):

```json
{
  "component": "timesheet-table",
  "label": "Time & Expenses",
  "properties": {
    "width": "col-12",
    "currencyCode": "USD",
    "toolbarActions": [{"name": "aiImport"}]
  }
}
```

When AI import is enabled:
- Users see an "Import with AI" button in the toolbar
- Three import modes: **Set Data** (replace), **Append Data** (add), **Fill Data** (fill gaps)
- Accepted files: `.pdf, .csv, .xls, .xlsx, .jpg, .jpeg, .png, .webp`
- Auto-maps columns: Name, Type, Rate, Quantity, Total (+ Tax Rate if `timesheetShowTax=true`)
- Can match imported items against configured service options

For service option matching during import, configure:
```json
{
  "timesheetServiceOptionsDataPath": "$.services",
  "timesheetServiceOptionsFilter": {"type": "equals(Billable)"},
  "timesheetServiceOptionsDataPathMapping": {
    "name": "serviceName",
    "type": "category",
    "rate": "price"
  }
}
```

## Complete Example

```json
{
  "component": "timesheet-table",
  "label": "Billing Timesheet",
  "required": true,
  "properties": {
    "width": "col-12",
    "currencyCode": "USD",
    "decimals": 2,
    "decimalsToInputAndSave": 2,
    "timesheetShowTax": true,
    "timesheetCreateButtonLabel": "Add Entry",
    "timesheetServiceOptionsDataPath": "$.rateCard",
    "timesheetServiceOptionsDataPathMapping": {
      "name": "serviceName",
      "type": "category",
      "rate": "hourlyRate",
      "taxRate": "vatRate"
    },
    "timesheetAddServiceOptions": ["ContractService", "WorkingHours"],
    "timesheetAddServiceOptionsLabels": {
      "ContractService": "Service Item",
      "WorkingHours": "Log Hours"
    },
    "minRange": 0,
    "maxRange": 24,
    "toolbarActions": [{"name": "aiImport"}]
  }
}
```

## Footer

The table footer displays:
- Tax total (if `timesheetShowTax=true`)
- Grand total with currency formatting

The total is automatically saved to `timesheetTotalDataPath` (defaults to `{dataPath}_Total`).
