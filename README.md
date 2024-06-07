## cue-bootstrap

Simple CLI tool which allows you transform set of JSON objects into the [CUE](https://github.com/cue-lang/cue) definition

Install it locally with command `go install github.com/sivukhin/cuebootstrap/cmd/cuebootstrap@latest`

### Example

If you have two json files:

- `a.json`

```json
{
  "Zero": 0,
  "NumberString": 42,
  "List": [1, 2, 3],
  "Object": {"Key": "cue", "Value": "awesome"}
}
```

- `b.json`

```json
{
  "Zero": 0,
  "NumberString": "string",
  "List": [42],
  "Object": {"Key": "json"}
}
```

You can create cue definition which will unify all JSON values with `cue-bootstrap` tool:
```bash
$> ./cue-bootstrap -inputs '*.json'
{
	Zero: number | *0
	List: [...number]
	NumberString: number | string
	Object: {
		Key:    string
		Value?: string | *"awesome"
	}
} 
```

You can also provide skeleton shape of the final schema with CUE definition.

For example, if you have the following JSON array
```json
[{"type":"insert","id":"1","value":"json"},{"type":"delete","id":"2"},{"type":"insert","id":"1","value":"cue","timeout":1000}]
```

You can guide `cueboostrap` into how you want your final schema looks like:
```
#item: type: string @discriminative()
#array: [...#item] @root()
```

Note, that you need to mark root schema definition with `@root()` tag. Also, you can mark string fields with `@discriminative()` tag in order to split schemas into multiple independent schemas based on the string value.

Finally, you will get following nice-looking schema

```cue
#item: {
    #insert: {
        id:       string
        value:    string
        timeout?: number
    }
    #delete: {
        id: string
    }
    type: "insert" | "delete"
    if type == "insert" {
        #insert
    }
    if type == "delete" {
        #delete
    }
}
#array: [...#item]
```