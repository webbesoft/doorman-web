<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up()
    {
        Schema::create('user_analytics', function (Blueprint $table) {
            $table->dropColumn('ip_hash');
            $table->dropUnique(['ip_hash', 'date']);
            $table->string('identifier');
            $table->string('identifier_type');

            $table->unique(['identifier', 'identifier_type', 'date']);
        });
    }

    public function down()
    {
        Schema::table('user_analytics', function (Blueprint $table) {
            $table->string('ip_hash');
            $table->dropUnique(['identifier', 'identifier_type', 'date']);
            $table->dropColumn('identifier');
            $table->dropColumn('identifier_type');

            $table->unique(['ip_hash', 'date']);
        });
    }
};
