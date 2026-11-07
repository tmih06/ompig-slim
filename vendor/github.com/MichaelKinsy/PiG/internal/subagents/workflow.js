// Native QuickJS adaptation of pi-subagents src/workflow/worker-source.ts (MIT).
// The host owns effects; these helpers preserve parallel/pipeline semantics.
const __RealDate = globalThis.Date;
const __die = what => { throw new Error(what + " is unavailable in workflow scripts (breaks resume)."); };
const Date = class extends __RealDate { static now() { return __die("Date.now()"); } constructor(...args) { if (!args.length) __die("new Date()"); super(...args); } };
const Math = Object.create(globalThis.Math);
Object.defineProperty(Math,"random",{value:()=>__die("Math.random()")});
const eval = () => __die("eval");
const Function = () => __die("Function");
let __spent = 0;
const budget = { total: null, spent: () => __spent, remaining: () => Infinity };
function __boundary(value, path = "the workflow result", seen = new Set()) {
  if (value === null || typeof value === "string" || typeof value === "boolean") return value;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (value === undefined && path === "the workflow result") return value;
  if (typeof value !== "object") throw new Error("Cannot pass " + typeof value + " across the workflow VM boundary (at " + path + ").");
  if (seen.has(value)) throw new Error("Cannot pass a circular structure across the workflow VM boundary.");
  seen.add(value);
  if (Object.getOwnPropertySymbols(value).length) throw new Error("Cannot pass symbol keys across the workflow VM boundary.");
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i++) { if (!Object.hasOwn(value,i)) throw new Error("Cannot pass a sparse array across the workflow VM boundary."); __boundary(value[i], path+"["+i+"]",seen); }
  } else {
    if (Object.getPrototypeOf(value) !== null && Object.getPrototypeOf(value) !== Object.prototype) throw new Error("Cannot pass a non-plain object across the workflow VM boundary.");
    for (const key of Object.keys(value)) __boundary(value[key],path+"."+key,seen);
  }
  seen.delete(value); return value;
}
function __text(value, what) { if (typeof value !== "string" || !value.trim()) throw new Error(what+" requires a non-empty string."); return value; }
function __optional(value,what) { return value == null ? undefined : __text(value,what); }
let __phase;
function phase(title) { __phase=__text(title,"phase(title)"); void tools.progress({type:"workflow_phase",title}); }
function log(message) { void tools.progress({type:"workflow_log",message:typeof message==="string"?message:JSON.stringify(message)}); }
async function agent(prompt, opts = {}) {
  __text(prompt,"agent(prompt)");
  if (opts === null) opts={};
  if (typeof opts!=="object" || Array.isArray(opts)) throw new Error("agent(prompt, opts) expects opts to be an object.");
  const keys=["label","phase","model","agentType","isolation","gate","resume","effort","schema"];
  for (const key of Object.keys(opts)) if (!keys.includes(key)) throw new Error("agent() opts."+key+" is not a recognised option. Supported: "+keys.join(", ")+".");
  const options={};
  for (const key of keys) if (key!=="schema" && opts[key]!=null) options[key]=__optional(opts[key],"agent() opts."+key);
  if (options.isolation!==undefined && options.isolation!=="worktree") throw new Error('agent() opts.isolation must be "worktree".');
  if (options.effort!==undefined && !["minimal","low","medium","high","xhigh","max"].includes(options.effort)) throw new Error("Invalid agent effort: "+options.effort);
  if (opts.schema!==undefined) {
    if (!opts.schema || typeof opts.schema!=="object" || Array.isArray(opts.schema)) throw new Error("agent() opts.schema must be a JSON Schema object.");
    options.schema=__boundary(opts.schema,"agent() opts.schema");
  }
  if (options.resume!==undefined) for (const key of ["agentType","model","effort","isolation","schema","gate"]) if (options[key]!==undefined) throw new Error("agent() opts.resume and opts."+key+" are mutually exclusive.");
  options.phase = options.phase || __phase;
  let response;
  try { response=await tools.agent({...options,prompt}); }
  catch(error) { if (/exceeded its cap/.test(error.message)) error.workflowFatal=true; throw error; }
  __spent=response.spent;
  return response.value;
}
function __list(list,what) { if (!Array.isArray(list)) throw new Error(what+" expects an array."); if (list.length>4096) throw new Error(what+" exceeds the item cap of 4096."); return list; }
function __fatal(error) { return !!error?.workflowFatal; }
async function parallel(thunks) {
  __list(thunks,"parallel(thunks)");
  for (const thunk of thunks) if (typeof thunk!=="function") throw new Error("parallel(thunks) expects an array of functions.");
  return Promise.all(thunks.map(async thunk => {try {return await thunk();}catch(e){if(__fatal(e))throw e;return null;}}));
}
async function pipeline(items,...stages) {
  __list(items,"pipeline(items, ...stages)");
  for (const stage of stages) if (typeof stage!=="function") throw new Error("pipeline stages must be functions.");
  return Promise.all(items.map(async (item,index) => {let value=item;for(const stage of stages){try{value=await stage(value,item,index);}catch(e){if(__fatal(e))throw e;return null;}}return value;}));
}
async function workflow(nameOrRef,args) {
  const ref=typeof nameOrRef==="string"?{name:__text(nameOrRef,"workflow(name)")}:nameOrRef;
  if (!ref || typeof ref!=="object" || (!ref.name&&!ref.scriptPath)) throw new Error("workflow() expects a name or {scriptPath}.");
  if(args!==undefined)__boundary(args,"workflow args");
  const response=await tools.workflow({...ref,args}); __spent=response.spent; return response.value;
}
