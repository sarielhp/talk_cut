#!/usr/bin/env ruby
# frozen_string_literal: true

require 'json'
require 'fileutils'

home = Dir.home
switcher_dir = File.join(home, '.config', 'opencode-switcher')
default_file = File.join(switcher_dir, 'default.json')

unless File.exist?(default_file)
  warn "Error: #{default_file} not found."
  exit 1
end

default_data = JSON.parse(File.read(default_file))
profile = default_data['default']

unless profile
  warn 'Error: No default profile found in default.json'
  exit 1
end

profile_dir = File.join(switcher_dir, profile)
key_file = File.join(profile_dir, 'API_key.sh')
conf_file = File.join(profile_dir, 'config.json')

api_key = nil
if File.exist?(key_file)
  content = File.read(key_file)
  api_key = Regexp.last_match(1) if content =~ /OPENROUTER_API_KEY=["']?([^"'\s]+)["']?/
end

model = 'google/gemini-2.5-flash-lite'
if File.exist?(conf_file)
  conf_data = JSON.parse(File.read(conf_file))
  target_model = conf_data['small_model'] || conf_data['model']
  model = target_model.sub(%r{\Aopenrouter/}, '') if target_model
end

target_dir = File.join(home, '.config', 'talk_cut')
FileUtils.mkdir_p(target_dir)

target_conf = File.join(target_dir, 'config.json')
existing_conf = {}
existing_conf = JSON.parse(File.read(target_conf)) if File.exist?(target_conf)

config_to_save = {
  'key_file' => existing_conf['key_file'] || '~/.config/auth/openrouter_api_key',
  'model' => model || existing_conf['model'] || 'google/gemini-2.5-flash-lite',
  'base_url' => existing_conf['base_url'] || 'https://openrouter.ai/api/v1',
  'default_privacy' => existing_conf['default_privacy'] || 'unlisted',
  'preferred_layout' => existing_conf['preferred_layout'] || 'slides',
  'youtube_secrets' => existing_conf['youtube_secrets'] || '~/.config/talk_cut/client_secrets.json'
}

File.write(target_conf, "#{JSON.pretty_generate(config_to_save)}\n")
File.chmod(0o600, target_conf)

puts "\e[32m✔ Successfully synced OpenRouter config from profile '#{profile}' to #{target_conf}\e[0m"
puts "  Model: #{config_to_save['model']}"
puts "  Key File: #{config_to_save['key_file']}"
exit 0
