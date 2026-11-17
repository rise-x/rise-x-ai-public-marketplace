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

For every property and its type, call `get_schema("components", "timesheet-table")`. The schema does not say:

- `currencyCode` defaults to `"USD"` on a new component.
- `timesheetTotalDataPath` defaults to `{dataPath}_Total` when unset.
- `minRange` / `maxRange` bound the hours per day on a `WorkingHours` entry (defaults 1 and 31).
- `timesheetAddServiceOptions` picks which service types the **Create** menu offers.

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

To enable AI import, add `{"name": "aiImport"}` to `toolbarActions`; the modes and file types are in `references/layouts-and-components.md` § AI Import. On a timesheet table, the import:

- Auto-maps the columns Name, Type, Rate, Quantity, Total (+ Tax Rate if `timesheetShowTax=true`).
- Matches imported items against the service options configured with `timesheetServiceOptionsDataPath`, `timesheetServiceOptionsFilter`, and `timesheetServiceOptionsDataPathMapping`.

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

The table footer displays the tax total (if `timesheetShowTax=true`) and the grand total with currency formatting. The grand total is saved to `timesheetTotalDataPath`.
