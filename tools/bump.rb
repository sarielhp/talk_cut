#!/usr/bin/env ruby
# frozen_string_literal: true

require 'open3'

root_dir = File.expand_path('..', __dir__)
Dir.chdir(root_dir)

# Parse bump mode: patch (default), minor, major
mode = 'patch'
custom_msg = nil

args = ARGV.dup
if args.first =~ /\A(patch|minor|major)\z/i
  mode = args.shift.downcase
end
custom_msg = args.join(' ').strip unless args.empty?

version_file = File.join(root_dir, 'VERSION')
unless File.exist?(version_file)
  warn "Error: VERSION file not found at #{version_file}"
  exit 1
end

current_version = File.read(version_file).strip
parts = current_version.split('.').map(&:to_i)
parts << 0 while parts.size < 3

case mode
when 'major'
  parts[0] += 1
  parts[1] = 0
  parts[2] = 0
when 'minor'
  parts[1] += 1
  parts[2] = 0
when 'patch'
  parts[2] += 1
else
  warn "Unknown bump mode: #{mode}. Use 'patch', 'minor', or 'major'."
  exit 1
end

new_version = parts.join('.')

# Update VERSION file
File.write(version_file, "#{new_version}\n")

# Update main.go Version literal if present
main_go = File.join(root_dir, 'main.go')
if File.exist?(main_go)
  content = File.read(main_go)
  content.sub!(/(Version\s*=\s*)"[^"]+"/, "\\1\"#{new_version}\"")
  File.write(main_go, content)
  Open3.capture3("gofmt -s -w #{main_go}")
end

# Check if quality gate can be skipped via .verified_head
verified_head_file = File.join(root_dir, '.verified_head')
can_skip_gate = false

if File.exist?(verified_head_file)
  verified_sha = File.read(verified_head_file).strip
  current_sha = `git rev-parse HEAD`.strip
  if verified_sha == current_sha
    status_out = `git status --porcelain`.strip
    modified_files = status_out.lines.map { |l| l.strip.split(/\s+/).last }
    allowed = ['VERSION', 'main.go']
    can_skip_gate = (modified_files - allowed).empty?
  end
end

gate_script = File.join(root_dir, 'tools', 'check.rb')
if !can_skip_gate && File.exist?(gate_script)
  system('ruby', gate_script)
  unless $CHILD_STATUS&.success? || $?.success?
    warn 'Quality gate failed during bump'
    exit 1
  end
end

# Stage and commit version bump
files_to_add = ['VERSION']
files_to_add << 'main.go' if File.exist?(main_go)
`git add #{files_to_add.join(' ')}`

commit_title = "chore: bump version to #{new_version} (#{mode})"
commit_title += " - #{custom_msg}" if custom_msg && !custom_msg.empty?

commit_out, commit_err, s = Open3.capture3('git', 'commit', '-m', commit_title)
unless s.success?
  warn 'git commit failed during bump'
  warn commit_out unless commit_out.empty?
  warn commit_err unless commit_err.empty?
  exit 1
end

# Push to origin if remote exists
has_origin = !`git remote get-url origin 2>/dev/null`.strip.empty?
if has_origin
  push_out, push_err, s = Open3.capture3('git push')
  unless s.success?
    warn 'git push failed'
    warn push_out unless push_out.empty?
    warn push_err unless push_err.empty?
    exit 1
  end
end

# Remove .verified_head after push
File.delete(verified_head_file) if File.exist?(verified_head_file)

# Install if make install is supported
if File.exist?(File.join(root_dir, 'Makefile')) && File.read(File.join(root_dir, 'Makefile')).include?('install:')
  system('make install')
end

puts "\e[32m✔ Bumped version from #{current_version} to #{new_version} [#{mode}]\e[0m"
exit 0
