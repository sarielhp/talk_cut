#!/usr/bin/env ruby
# frozen_string_literal: true

require 'open3'

root_dir = File.expand_path('..', __dir__)
Dir.chdir(root_dir)

# Check if there are uncommitted changes
status_out, _, _ = Open3.capture3('git status --porcelain')
changed_lines = status_out.lines.map(&:strip).reject(&:empty?)

if changed_lines.empty?
  puts "Nothing to commit, working tree clean."
  exit 0
end

msg = ARGV.join(' ').strip

# Auto-generate a descriptive WIP summary if no message was supplied
if msg.empty?
  file_paths = changed_lines.map { |l| l.sub(/\A\S+\s+/, '').strip }
  dirs = file_paths.map do |path|
    dir = File.dirname(path)
    dir == '.' ? File.basename(path) : dir
  end.uniq

  summary_target = dirs.first(3).join(', ')
  summary_target += " (+#{dirs.size - 3} more)" if dirs.size > 3
  msg = "wip: update #{summary_target} (#{file_paths.size} file#{file_paths.size == 1 ? '' : 's'})"
end

# 1. Quality gate
gate_script = File.join(root_dir, 'tools', 'check.rb')
if File.exist?(gate_script)
  system('ruby', gate_script)
  exit 1 unless $CHILD_STATUS&.success? || $?.success?
end

# 2. Stage changes
_, err, s = Open3.capture3('git add -A')
unless s.success?
  warn "git add failed: #{err}"
  exit 1
end

# 3. Commit
out, err, s = Open3.capture3('git', 'commit', '-m', msg)
unless s.success?
  warn out unless out.empty?
  warn err unless err.empty?
  exit 1
end

# 4. Record .verified_head
head = `git rev-parse HEAD`.strip
File.write(File.join(root_dir, '.verified_head'), head)

puts "\n\e[32m✔ Successfully verified and committed:\e[0m #{msg}"
exit 0
