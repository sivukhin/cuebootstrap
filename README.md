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
$ ./cue-bootstrap -inputs '*.json'
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
