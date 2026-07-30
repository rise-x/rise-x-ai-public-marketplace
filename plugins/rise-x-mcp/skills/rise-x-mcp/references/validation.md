# Validation

## Contents

- [Overview](#overview)
- [Validation Rule Structure](#validation-rule-structure)
  - [Rule String Syntax](#rule-string-syntax)
  - [Parameter Types](#parameter-types)
  - [Conditional Suffixes](#conditional-suffixes)
- [`required` Property vs. `required` Rule](#required-property-vs-required-rule)
- [`alwaysTrigger` Explained](#alwaystrigger-explained)
- [Rule Reference](#rule-reference)
  - [Required & Conditional Required](#required--conditional-required)
  - [Disabled (Conditional)](#disabled-conditional)
  - [Numeric](#numeric)
  - [Comparison](#comparison)
  - [String](#string)
  - [Array](#array)
  - [Date](#date)
  - [Boolean](#boolean)
  - [Time Range](#time-range)
  - [Custom Expression](#custom-expression)
- [Data-Grid Column Validation](#data-grid-column-validation)
- [Practical Examples](#practical-examples)
- [Error Display Behavior](#error-display-behavior)

## Overview

Rise-X supports declarative validation rules on layout components and data-grid columns. Rules are defined in the `properties.validation` array on a component and evaluated at runtime against work data. Validation errors block task submission; warnings are advisory.

## Validation Rule Structure

Each rule in the `properties.validation` array is an object:

```json
{
  "type": "error",
  "rules": "required|min:100",
  "message": "Volume must be at least 100 MT",
  "alwaysTrigger": false
}
```

| Field | Type | Description |
|---|---|---|
| `type` | `"error"` \| `"warning"` \| `"success"` | **error** blocks submission, **warning** is advisory, **success** shows positive feedback |
| `rules` | `string` | Pipe-separated rule definitions (see rule reference below) |
| `message` | `string` | Message displayed when validation fails |
| `alwaysTrigger` | `boolean` | If `true`, message shows even when validation passes. Useful for persistent warnings or info. Default `false`. |

### Rule String Syntax

Rules are **pipe-separated** (`|`). Each rule is `ruleName` or `ruleName:param` or `ruleName:param1,param2`.

```
"required"                              # single rule
"required|numeric|min:0"                # multiple rules
"requiredIf:$.data.status|size:5"       # rule with data path parameter
"regExp:^[A-Z]{3}-\\d{4}$"             # regex rule with pattern
```

### Parameter Types

Rule parameters can reference:

| Prefix | Meaning | Example |
|---|---|---|
| `$.path` | JSON data path in work data | `requiredIf:$.data.status` |
| `fieldName` | Another component's name in the same layout | `same:confirmEmail` |
| `@row` | Current row in data-grid | `requiredWith:@row.quantity` |
| `@rowPrev` | Previous row in data-grid | `greaterThan:@rowPrev.date` |
| `@rowNext` | Next row in data-grid | `lessThan:@rowNext.date` |
| `@value` | Hard-coded value | `min:@100` |
| `=expression` | Dynamic expression | `expression:=$.data.age > 18` |

### Conditional Suffixes

Some rules support conditional execution:

- `IfCurrentValueExists` — only validate if the current field has a value
- `IfLinkedValueExists` — only validate if the compared field has a value

Example: `afterIfCurrentValueExists:$.startDate` — validates only when both dates are present.

## `required` Property vs. `required` Rule

Two ways to mark a field as required:

1. **Top-level `required: true` property** on the component — adds an automatic "This field is required" error rule if no validation array exists. If a validation array IS present, it checks whether a `required` rule already exists in it.

2. **`required` rule in validation array** — explicit rule with custom message:
   ```json
   {"type": "error", "rules": "required", "message": "Concentrate grade is mandatory"}
   ```

For simple required fields, just use `required: true` on the component. For custom messages or combined rules, use the validation array.

## `alwaysTrigger` Explained

When `alwaysTrigger: true`:
- The validation message displays **even when the field is valid**
- Useful for persistent informational warnings (e.g. "Prices over $1000/MT may require additional approval")
- The validation result stays in the UI and is not cleared on successful validation
- Typically used with `type: "warning"`, not `type: "error"`

## Rule Reference

### Required & Conditional Required

| Rule | Description | Example |
|---|---|---|
| `required` | Field must have a value | `required` |
| `requiredIf:param` | Required if referenced field/expression is truthy | `requiredIf:$.data.needsApproval` |
| `requiredWith:p1,p2` | Required if **any** listed field has a value | `requiredWith:$.data.price,$.data.qty` |
| `requiredWithAll:p1,p2` | Required if **all** listed fields have values | `requiredWithAll:$.data.startDate,$.data.endDate` |
| `requiredWithout:p1,p2` | Required if **any** listed field is empty | `requiredWithout:$.data.email,$.data.phone` |
| `requiredWithoutAll:p1,p2` | Required if **all** listed fields are empty | `requiredWithoutAll:$.data.fax,$.data.telex` |
| `requiredWithAllConfirmed:p` | Required if all fields are confirmed (true/1/on/yes) | `requiredWithAllConfirmed:$.data.termsAccepted` |
| `requiredWithoutAllConfirmed:p` | Required if all fields are NOT confirmed | `requiredWithoutAllConfirmed:$.data.override` |

### Disabled (Conditional)

| Rule | Description | Example |
|---|---|---|
| `disabledIf:param` | Disabled if expression is truthy | `disabledIf:$.data.isLocked` |
| `disabledWith:p1,p2` | Disabled if **any** listed field has value | `disabledWith:$.data.autoCalculated` |
| `disabledWithAll:p1,p2` | Disabled if **all** listed fields have values | `disabledWithAll:$.data.a,$.data.b` |
| `disabledWithout:p1,p2` | Disabled if **any** listed field is empty | `disabledWithout:$.data.prerequisite` |
| `disabledWithoutAll:p1,p2` | Disabled if **all** listed fields are empty | `disabledWithoutAll:$.data.x,$.data.y` |

### Numeric

| Rule | Description | Example |
|---|---|---|
| `integer` | Must be an integer | `integer` |
| `float` | Must be a float | `float` |
| `numeric` | Must be numeric | `numeric` |
| `decimal:from,to` | Decimal places between from and to | `decimal:2,4` |
| `digits:length` | Exact digit count | `digits:7` |
| `digitsBetween:min,max` | Digit count between min and max | `digitsBetween:5,10` |
| `min:value` | Minimum value (numeric) or length (string) | `min:100` |
| `max:value` | Maximum value or length | `max:10000` |

### Comparison

| Rule | Description | Example |
|---|---|---|
| `lessThan:field` | Less than another field's value | `lessThan:$.data.maxPrice` |
| `lessThanOrEqual:field` | Less than or equal | `lessThanOrEqual:$.data.budget` |
| `greaterThan:field` | Greater than another field's value | `greaterThan:$.data.minOrder` |
| `greaterThanOrEqual:field` | Greater than or equal | `greaterThanOrEqual:$.data.threshold` |
| `same:field` | Must match another field | `same:confirmEmail` |
| `different:field` | Must differ from another field | `different:$.data.oldPassword` |
| `size:value` | Exact length/count match | `size:5` |
| `sizeBetween:min,max` | Length/count between min and max | `sizeBetween:3,50` |

### String

| Rule | Description | Example |
|---|---|---|
| `startsWith:v1,v2` | Must start with one of the values | `startsWith:IMO,MMSI` |
| `endsWith:v1,v2` | Must end with one of the values | `endsWith:.pdf,.docx` |
| `regExp:pattern` | Must match regular expression | `regExp:^[A-Z]{2}\\d{6}$` |
| `url` | Must be a valid URL | `url` |
| `in:v1,v2,v3` | Must be one of the listed values | `in:CIF,FOB,CFR` |
| `inArray:field` | Must exist in another field's array | `inArray:$.data.allowedPorts` |

### Array

| Rule | Description | Example |
|---|---|---|
| `array` | Must be an array | `array` |
| `isUniqueInArray:path` | Value must be unique within an array property | `isUniqueInArray:$.data.items.[*].sku` |

### Date

| Rule | Description | Example |
|---|---|---|
| `before:field` | Before another date field | `before:$.data.endDate` |
| `beforeOrEqual:field` | Before or equal to another date | `beforeOrEqual:$.data.deadline` |
| `beforeNow` | Before current timestamp | `beforeNow` |
| `beforeToday` | Before end of today | `beforeToday` |
| `beforeOrEqualToday` | Before or equal to end of today | `beforeOrEqualToday` |
| `after:field` | After another date field | `after:$.data.startDate` |
| `afterOrEqual:field` | After or equal to another date | `afterOrEqual:$.data.minDate` |
| `afterNow` | After current timestamp | `afterNow` |
| `afterToday` | After start of today | `afterToday` |
| `afterOrEqualToday` | After or equal to start of today | `afterOrEqualToday` |
| `dateEquals:field` | Exact date match | `dateEquals:$.data.targetDate` |
| `beforeIfExists:field` | Before, only if reference date exists | `beforeIfExists:$.data.endDate` |
| `afterIfExists:field` | After, only if reference date exists | `afterIfExists:$.data.startDate` |
| `beforeOrEqualIfExists:field` | Before/equal, only if exists | `beforeOrEqualIfExists:$.data.deadline` |

### Boolean

| Rule | Description | Example |
|---|---|---|
| `boolean` | Must be boolean-like (true/false/1/0/on/off/yes/no) | `boolean` |
| `confirmed` | Must be confirmed (true/1/on/yes) | `confirmed` |
| `declined` | Must be declined (false/0/off/no) | `declined` |

### Time Range

| Rule | Description | Example |
|---|---|---|
| `isWithinHourRange:from,to` | Time within hour range | `isWithinHourRange:8,17` |
| `exceptHourRange:from,to` | Time outside hour range | `exceptHourRange:0,6` |

### Custom Expression

| Rule | Description | Example |
|---|---|---|
| `expression:expr` | Custom expression — error when expression returns **false** | `expression:=$.data.qty * $.data.price < 1000000` |

## Data-Grid Column Validation

Data-grid columns support the same validation rules as regular components. Define them in the column's `validation` property:

```json
{
  "component": "data-grid",
  "label": "Line Items",
  "properties": {
    "columns": [
      {
        "name": "sku",
        "type": "string",
        "displayName": "SKU",
        "validation": [
          {"type": "error", "rules": "required|regExp:^[A-Z]{3}-\\d{4}$", "message": "SKU must be format AAA-0000"}
        ]
      },
      {
        "name": "quantity",
        "type": "number",
        "displayName": "Quantity",
        "validation": [
          {"type": "error", "rules": "required|numeric|min:1", "message": "Quantity must be at least 1"}
        ]
      },
      {
        "name": "unitPrice",
        "type": "currency",
        "displayName": "Unit Price",
        "validation": [
          {"type": "error", "rules": "required|numeric|min:0.01", "message": "Price must be positive"},
          {"type": "warning", "rules": "max:10000", "message": "High unit price — verify before submitting", "alwaysTrigger": false}
        ]
      }
    ]
  }
}
```

Use `@row`, `@rowPrev`, `@rowNext` to reference row-level data in grid rules.

## Practical Examples

### Simple required with custom message
```json
{
  "component": "input-text",
  "label": "Vessel Name",
  "properties": {
    "width": "col-6",
    "validation": [
      {"type": "error", "rules": "required", "message": "Vessel name is required"}
    ]
  }
}
```

### Numeric range with unit
```json
{
  "component": "input-text",
  "label": "Volume",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "unitOfMeasure": "MT",
    "validation": [
      {"type": "error", "rules": "required|numeric|min:1|max:100000", "message": "Volume must be between 1 and 100,000 MT"}
    ]
  }
}
```

### Date range validation (start before end)
```json
// Start Date component
{
  "component": "date-picker",
  "label": "Delivery Window From",
  "dataPath": "$.requirement.deliveryFrom",
  "properties": {
    "width": "col-6",
    "validation": [
      {"type": "error", "rules": "required|beforeOrEqual:$.requirement.deliveryTo", "message": "Start date must be before or equal to end date"}
    ]
  }
}

// End Date component
{
  "component": "date-picker",
  "label": "Delivery Window To",
  "dataPath": "$.requirement.deliveryTo",
  "properties": {
    "width": "col-6",
    "validation": [
      {"type": "error", "rules": "required|afterOrEqual:$.requirement.deliveryFrom|afterToday", "message": "End date must be after start date and in the future"}
    ]
  }
}
```

### Conditional required (required only when another field has value)
```json
{
  "component": "input-text",
  "label": "Counter-Offer Price",
  "dataPath": "$.review.counterPrice",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'",
    "validation": [
      {"type": "error", "rules": "requiredIf:$.review.decision|numeric|min:0.01", "message": "Price is required for counter-offers"}
    ]
  }
}
```

### Warning (advisory, non-blocking)
```json
{
  "component": "input-text",
  "label": "Price Per Ton",
  "dataPath": "$.offer.pricePerTon",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "unitOfMeasure": "USD/MT",
    "validation": [
      {"type": "error", "rules": "required|numeric|min:0.01", "message": "Price is required"},
      {"type": "warning", "rules": "max:5000", "message": "Price exceeds $5,000/MT — please verify before submitting"}
    ]
  }
}
```

### Regex pattern validation
```json
{
  "component": "input-text",
  "label": "IMO Number",
  "properties": {
    "width": "col-6",
    "validation": [
      {"type": "error", "rules": "required|regExp:^\\d{7}$", "message": "IMO number must be exactly 7 digits"}
    ]
  }
}
```

### Multiple rules with conditional required and comparison
```json
{
  "component": "input-text",
  "label": "Offered Volume",
  "dataPath": "$.offer.volume",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "unitOfMeasure": "MT",
    "validation": [
      {"type": "error", "rules": "required|numeric|min:1|lessThanOrEqual:$.requirement.volume", "message": "Offered volume must be positive and not exceed requested volume"}
    ]
  }
}
```

### Custom expression validation
```json
{
  "component": "input-text",
  "label": "Total Value",
  "dataPath": "$.order.totalValue",
  "readOnly": true,
  "defaultValue": "={$.order.quantity} * {$.order.unitPrice}",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "validation": [
      {"type": "error", "rules": "expression:=$.order.totalValue < 1000000", "message": "Order value cannot exceed $1,000,000 without executive approval"}
    ]
  }
}
```

## Error Display Behavior

- **Error** (`type: "error"`) — Red border, red background, blocks task submission
- **Warning** (`type: "warning"`) — Orange border, orange background, does NOT block submission
- **Success** (`type: "success"`) — No visual indicator (transparent)

Validation messages appear as tooltips on hover over the field. When submission is attempted with errors, the UI scrolls to the first invalid field.
