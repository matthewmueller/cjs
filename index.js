module.exports = { "ab cd": foo }
exports["not identifier"] = "asdf"
exports["\u{D83C}\u{DF10}"] = 1
exports["\u{D83C}"] = 1
exports["\u58b8"] = 1
exports["\n"] = 1
exports["\xFF"] = 1
exports["\x09"] = 1
exports["\x03z"] = 1
exports["'"] = 1
exports["@notidentifier"] = "asdf"
Object.defineProperty(exports, "%notidentifier", { value: x })
Object.defineProperty(exports, "hm🤔", { value: x })
exports["⨉"] = 45
exports["α"] = 54
exports.package = "STRICT RESERVED!"
exports.var = "RESERVED"
