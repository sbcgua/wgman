# Porting WGMAN to Rust

I want to port this tool to Rust. Read the docs/SPEC.md to understand the purpose of the tool and the original design. Also read docs/NOTES.md - these are durable implementation conventions, that could also be useful.

After that, let's discuss the plan. Use /grilling to discuss the details with me before the implementation. Don't start the implementation yet until I confirm. After the interview create a PLAN.md in the root dir, with reasonably sized phases, so that they can be delivered by separately spawned subagents.

General considerations:

- Try keeping the architecture close to the golang version. Yet adapt it to the ideomatic approachs in Rust.
- The implementation will be done in a dev container that lacks ipset, iptables and wg. Use unit tests to prove the functionality. The test on a real system will be done separately.
- When comes to implementation
  - Do not overwrite the code in the original directory and don't touch original files. Instead, create a `rust-port` directory and suppose this is the root for the Rust project from scratch.
  - In particular create, `docs` subdir there and maintain own rust-specific NOTES.md there
- Plan the unit tests coverage comparable to the orginal code
